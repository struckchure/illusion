// Implements glue.h on top of Jolt's C++ API.

#include "glue.h"

#include <Jolt/Jolt.h>

#include <Jolt/Core/Factory.h>
#ifdef __EMSCRIPTEN__
#include <Jolt/Core/JobSystemSingleThreaded.h>
#else
#include <Jolt/Core/JobSystemThreadPool.h>
#endif
#include <Jolt/Core/TempAllocator.h>
#include <Jolt/Physics/Body/BodyCreationSettings.h>
#include <Jolt/Physics/Body/BodyLock.h>
#include <Jolt/Physics/Character/CharacterVirtual.h>
#include <Jolt/Physics/Collision/CastResult.h>
#include <Jolt/Physics/Collision/CollisionCollectorImpl.h>
#include <Jolt/Physics/Collision/ShapeCast.h>
#include <Jolt/Physics/Collision/ContactListener.h>
#include <Jolt/Physics/Collision/RayCast.h>
#include <Jolt/Physics/Collision/Shape/BoxShape.h>
#include <Jolt/Physics/Collision/Shape/CapsuleShape.h>
#include <Jolt/Physics/Collision/Shape/ConvexHullShape.h>
#include <Jolt/Physics/Collision/Shape/CylinderShape.h>
#include <Jolt/Physics/Collision/Shape/MeshShape.h>
#include <Jolt/Physics/Collision/Shape/OffsetCenterOfMassShape.h>
#include <Jolt/Physics/Collision/Shape/RotatedTranslatedShape.h>
#include <Jolt/Physics/Collision/Shape/SphereShape.h>
#include <Jolt/Physics/PhysicsSettings.h>
#include <Jolt/Physics/PhysicsSystem.h>
#include <Jolt/Physics/Vehicle/MotorcycleController.h>
#include <Jolt/Physics/Vehicle/VehicleCollisionTester.h>
#include <Jolt/Physics/Vehicle/VehicleConstraint.h>
#include <Jolt/Physics/Vehicle/WheeledVehicleController.h>
#include <Jolt/RegisterTypes.h>

#include <algorithm>
#include <cstddef>
#include <mutex>
#include <thread>
#include <unordered_map>
#include <vector>

using namespace JPH;

namespace {

// All moving roles share a broad phase. Characters yield to vehicles,
// while other body pairs retain their normal response.
constexpr ObjectLayer kNonMoving = 0;
constexpr ObjectLayer kMoving = 1;
constexpr ObjectLayer kCharacter = 2;
constexpr ObjectLayer kVehicle = 3;

class WheelLayers final : public ObjectLayerFilter {
public:
	bool ShouldCollide(ObjectLayer layer) const override { return layer != kCharacter; }
};
const WheelLayers kWheelLayers;
constexpr BroadPhaseLayer kBPNonMoving(0);
constexpr BroadPhaseLayer kBPMoving(1);

class BroadPhaseLayers final : public BroadPhaseLayerInterface {
public:
	uint GetNumBroadPhaseLayers() const override { return 2; }
	BroadPhaseLayer GetBroadPhaseLayer(ObjectLayer layer) const override {
		return layer == kNonMoving ? kBPNonMoving : kBPMoving;
	}
#if defined(JPH_EXTERNAL_PROFILE) || defined(JPH_PROFILE_ENABLED)
	const char* GetBroadPhaseLayerName(BroadPhaseLayer layer) const override {
		return layer == kBPNonMoving ? "NonMoving" : "Moving";
	}
#endif
};

class ObjectVsBroadPhase final : public ObjectVsBroadPhaseLayerFilter {
public:
	bool ShouldCollide(ObjectLayer layer, BroadPhaseLayer bp) const override {
		return layer == kNonMoving ? bp == kBPMoving : true;
	}
};

class ObjectPairs final : public ObjectLayerPairFilter {
public:
	bool ShouldCollide(ObjectLayer a, ObjectLayer b) const override {
		return a != kNonMoving || b != kNonMoving;
	}
};

// Buffers contact begin/end events, one pair of events per pair of bodies.
//
// Jolt reports contacts per sub-shape pair (each triangle of a mesh, say), so
// the buffer tracks every live sub-shape contact and only reports a body
// pair's first beginning and last ending. Jolt also reports a body's contacts
// as removed when it falls asleep (and as added when it wakes), so removals
// are resolved after each step: contacts of sleeping bodies are kept, and are
// ended only if the body wakes up somewhere else. Jolt calls the listener
// methods from its worker threads.
class ContactBuffer final : public ContactListener {
public:
	struct SubKey {
		uint32_t body1, sub1, body2, sub2;
		bool operator==(const SubKey& o) const {
			return body1 == o.body1 && sub1 == o.sub1 && body2 == o.body2 && sub2 == o.sub2;
		}
	};

	void OnContactAdded(const Body& b1, const Body& b2, const ContactManifold& m, ContactSettings& settings) override {
		characterResponse(b1, b2, settings);
		RVec3 p = m.mRelativeContactPointsOn1.empty() ? m.mBaseOffset : m.GetWorldSpaceContactPointOn1(0);
		float point[3] = {float(p.GetX()), float(p.GetY()), float(p.GetZ())};
		float normal[3] = {m.mWorldSpaceNormal.GetX(), m.mWorldSpaceNormal.GetY(), m.mWorldSpaceNormal.GetZ()};
		Add({b1.GetID().GetIndexAndSequenceNumber(), m.mSubShapeID1.GetValue(),
				b2.GetID().GetIndexAndSequenceNumber(), m.mSubShapeID2.GetValue()},
			point, normal);
	}

	void OnContactPersisted(const Body& b1, const Body& b2, const ContactManifold&, ContactSettings& settings) override {
		characterResponse(b1, b2, settings);
	}

	void OnContactRemoved(const SubShapeIDPair& pair) override {
		std::lock_guard<std::mutex> lock(mu);
		removed.push_back({pair.GetBody1ID().GetIndexAndSequenceNumber(), pair.GetSubShapeID1().GetValue(),
			pair.GetBody2ID().GetIndexAndSequenceNumber(), pair.GetSubShapeID2().GetValue()});
	}

	// Add records a sub-shape contact, reporting a beginning if it is the
	// body pair's first.
	void Add(const SubKey& k, const float point[3], const float normal[3]) {
		std::lock_guard<std::mutex> lock(mu);
		auto [it, inserted] = subs.try_emplace(k);
		it->second = SubState{false, true, 0};
		if (!inserted || pairs[pairKey(k.body1, k.body2)]++ > 0) {
			return;
		}
		ILL_ContactEvent e{};
		e.body1 = k.body1;
		e.body2 = k.body2;
		e.kind = ILL_CONTACT_BEGIN;
		std::copy(point, point + 3, e.point);
		std::copy(normal, normal + 3, e.normal);
		events.push_back(e);
	}

	// RemoveNow drops a sub-shape contact immediately.
	void RemoveNow(const SubKey& k) {
		std::lock_guard<std::mutex> lock(mu);
		erase(k);
	}

	// Resolve processes the removals Jolt reported during a step. It runs
	// after the step, when bodies can be inspected.
	void Resolve(BodyInterface& bi) {
		std::lock_guard<std::mutex> lock(mu);
		for (const SubKey& k : removed) {
			auto it = subs.find(k);
			if (it == subs.end()) {
				continue;
			}
			if (asleep(bi, k)) {
				it->second.asleep = true;
				it->second.missed = 0;
			} else {
				erase(k);
			}
		}
		removed.clear();

		// A sleeping contact whose bodies woke up is re-added by Jolt if they
		// still touch; give it a step's grace, then end it.
		scratch.clear();
		for (auto& [k, st] : subs) {
			if (st.asleep && !asleep(bi, k)) {
				if (st.seen) {
					st.asleep = false;
				} else if (++st.missed >= 2) {
					scratch.push_back(k);
				}
			}
			st.seen = false;
		}
		for (const SubKey& k : scratch) {
			erase(k);
		}
	}

	// EndAll ends every contact of a body that is about to be destroyed, since
	// Jolt can't report them once it is gone.
	void EndAll(uint32_t body) {
		std::lock_guard<std::mutex> lock(mu);
		scratch.clear();
		for (const auto& [k, st] : subs) {
			if (k.body1 == body || k.body2 == body) {
				scratch.push_back(k);
			}
		}
		for (const SubKey& k : scratch) {
			erase(k);
		}
	}

	int Drain(ILL_ContactEvent* out, int cap) {
		std::lock_guard<std::mutex> lock(mu);
		int n = std::min<int>(cap, int(events.size() - head));
		std::copy(events.begin() + head, events.begin() + head + n, out);
		head += n;
		if (head == events.size()) {
			events.clear();
			head = 0;
		}
		return n;
	}

private:
	static void characterResponse(const Body& b1, const Body& b2, ContactSettings& settings) {
		// The character receives the response; neither contact torque nor
		// penetration recovery may rotate the vehicle.
		if (b1.GetObjectLayer() == kVehicle && b2.GetObjectLayer() == kCharacter) {
			settings.mInvMassScale1 = settings.mInvInertiaScale1 = 0;
		} else if (b2.GetObjectLayer() == kVehicle && b1.GetObjectLayer() == kCharacter) {
			settings.mInvMassScale2 = settings.mInvInertiaScale2 = 0;
		}
	}

	struct SubState {
		bool asleep; // removed by Jolt because a body fell asleep
		bool seen;   // added this step
		int missed;  // steps awake without being re-added
	};

	struct SubKeyHash {
		size_t operator()(const SubKey& k) const {
			uint64_t h = (uint64_t(k.body1) << 32 | k.body2) * 0x9E3779B97F4A7C15ull;
			return size_t(h ^ ((uint64_t(k.sub1) << 32 | k.sub2) + 0x7F4A7C159E3779B9ull + (h << 6) + (h >> 2)));
		}
	};

	static uint64_t pairKey(uint32_t a, uint32_t b) {
		if (a > b) {
			std::swap(a, b);
		}
		return uint64_t(a) << 32 | b;
	}

	// asleep reports whether a moving body of the pair is sleeping. Static
	// bodies never count: they are never active.
	static bool asleep(BodyInterface& bi, const SubKey& k) {
		for (uint32_t raw : {k.body1, k.body2}) {
			BodyID id(raw);
			if (bi.IsAdded(id) && bi.GetMotionType(id) != EMotionType::Static && !bi.IsActive(id)) {
				return true;
			}
		}
		return false;
	}

	// erase drops a sub-shape contact, reporting an ending if it was the body
	// pair's last. Callers hold mu.
	void erase(const SubKey& k) {
		if (subs.erase(k) == 0) {
			return;
		}
		auto p = pairs.find(pairKey(k.body1, k.body2));
		if (p == pairs.end() || --p->second > 0) {
			return;
		}
		pairs.erase(p);
		ILL_ContactEvent e{};
		e.body1 = k.body1;
		e.body2 = k.body2;
		e.kind = ILL_CONTACT_END;
		events.push_back(e);
	}

	std::mutex mu;
	std::vector<ILL_ContactEvent> events;
	size_t head = 0; // events before head have been drained
	std::unordered_map<SubKey, SubState, SubKeyHash> subs;
	std::unordered_map<uint64_t, int> pairs; // live sub-shape contacts per body pair
	std::vector<SubKey> removed;            // reported during the current step
	std::vector<SubKey> scratch;
};

// Reports a character's contacts with static and kinematic bodies. (Its inner
// body already produces contacts with dynamic bodies and sensors through the
// regular listener, and Jolt doesn't generate kinematic-vs-static contacts.)
class CharacterContacts final : public CharacterContactListener {
public:
	CharacterContacts(PhysicsSystem& system, ContactBuffer& buffer) : system(system), buffer(buffer) {}

	void OnContactAdded(const CharacterVirtual* ch, const BodyID& other, const SubShapeID& sub, RVec3Arg point,
		Vec3Arg normal, CharacterContactSettings& settings) override {
		yieldToVehicle(other, settings);
		if (!reportable(other)) {
			return;
		}
		float p[3] = {float(point.GetX()), float(point.GetY()), float(point.GetZ())};
		float n[3] = {normal.GetX(), normal.GetY(), normal.GetZ()};
		buffer.Add(key(ch, other, sub), p, n);
	}

	void OnContactPersisted(const CharacterVirtual*, const BodyID& other, const SubShapeID&, RVec3Arg, Vec3Arg, CharacterContactSettings& settings) override {
		yieldToVehicle(other, settings);
	}

	void OnContactRemoved(const CharacterVirtual* ch, const BodyID& other, const SubShapeID& sub) override {
		if (!reportable(other)) {
			return;
		}
		buffer.RemoveNow(key(ch, other, sub));
	}

private:
	void yieldToVehicle(const BodyID& other, CharacterContactSettings& settings) {
		BodyLockRead lock(system.GetBodyLockInterfaceNoLock(), other);
		if (lock.Succeeded() && lock.GetBody().GetObjectLayer() == kVehicle) {
			settings.mCanReceiveImpulses = false;
		}
	}

	static ContactBuffer::SubKey key(const CharacterVirtual* ch, const BodyID& other, const SubShapeID& sub) {
		return {ch->GetInnerBodyID().GetIndexAndSequenceNumber(), 0, other.GetIndexAndSequenceNumber(), sub.GetValue()};
	}

	bool reportable(const BodyID& id) {
		BodyLockRead lock(system.GetBodyLockInterfaceNoLock(), id);
		return lock.Succeeded() && !lock.GetBody().IsDynamic() && !lock.GetBody().IsSensor();
	}

	PhysicsSystem& system;
	ContactBuffer& buffer;
};

Vec3 vec3(const float* v) { return Vec3(v[0], v[1], v[2]); }
RVec3 rvec3(const float* v) { return RVec3(v[0], v[1], v[2]); }
Quat quat(const float* v) {
	Quat q(v[0], v[1], v[2], v[3]);
	return q.LengthSq() > 0 ? q.Normalized() : Quat::sIdentity();
}
void put(Vec3Arg v, float* out) {
	out[0] = v.GetX();
	out[1] = v.GetY();
	out[2] = v.GetZ();
}
void putR(RVec3Arg v, float* out) {
	out[0] = float(v.GetX());
	out[1] = float(v.GetY());
	out[2] = float(v.GetZ());
}
void putQ(QuatArg q, float* out) {
	out[0] = q.GetX();
	out[1] = q.GetY();
	out[2] = q.GetZ();
	out[3] = q.GetW();
}

ILL_Shape* keep(const ShapeSettings::ShapeResult& r) {
	if (r.HasError()) {
		return nullptr;
	}
	const Shape* s = r.Get().GetPtr();
	s->AddRef();
	return reinterpret_cast<ILL_Shape*>(const_cast<Shape*>(s));
}

const Shape* shapeOf(ILL_Shape* s) { return reinterpret_cast<const Shape*>(s); }

// scaled is a friction curve with its friction (Y) values scaled by k.
LinearCurve scaled(const LinearCurve& c, float k) {
	LinearCurve out;
	out.Reserve(uint(c.mPoints.size()));
	for (const LinearCurve::Point& p : c.mPoints) {
		out.AddPoint(p.mX, p.mY * k);
	}
	return out;
}

// Jolt requires the convex radius to fit inside the shape.
float convexRadius(float smallest) { return std::min(cDefaultConvexRadius, smallest * 0.5f); }

} // namespace

struct ILL_World {
	BroadPhaseLayers bpLayers;
	ObjectVsBroadPhase objectVsBP;
	ObjectPairs pairs;
	ContactBuffer contacts;
	TempAllocatorImpl temp{32 * 1024 * 1024};
#ifdef __EMSCRIPTEN__
	// Browser builds have no threads (they'd need SharedArrayBuffer and
	// cross-origin isolation headers), so jobs run on the calling thread.
	JobSystemSingleThreaded jobs{cMaxPhysicsJobs};
#else
	JobSystemThreadPool jobs{cMaxPhysicsJobs, cMaxPhysicsBarriers,
		int(std::max(1u, std::thread::hardware_concurrency()) - 1)};
#endif
	// Declared after everything it refers to, so it is destroyed first.
	PhysicsSystem system;
	CharacterContacts characterContacts{system, contacts};

	struct Vehicle {
		Ref<VehicleConstraint> constraint;
		std::vector<Vec3> right, up; // per wheel, for read-back
	};
	std::unordered_map<uint32_t, Vehicle> vehicles; // by body

	// removeVehicle takes a body's vehicle out of the simulation and frees it.
	void removeVehicle(uint32_t body) {
		auto it = vehicles.find(body);
		if (it == vehicles.end()) {
			return;
		}
		system.RemoveStepListener(it->second.constraint);
		system.RemoveConstraint(it->second.constraint);
		vehicles.erase(it);
		system.GetBodyInterface().SetObjectLayer(BodyID(body), kMoving);
	}
};

struct ILL_Character {
	ILL_World* world;
	Ref<CharacterVirtual> ch;
};

#ifdef __EMSCRIPTEN__
// jolt_js.go mirrors these structs; keep the layouts in sync.
static_assert(sizeof(ILL_BodySettings) == 112 && offsetof(ILL_BodySettings, userData) == 104, "jolt_js.go cBodySettings");
static_assert(sizeof(ILL_ContactEvent) == 36, "jolt_js.go cContactEvent");
static_assert(sizeof(ILL_RayHit) == 32, "jolt_js.go cRayHit");
static_assert(sizeof(ILL_WheelDesc) == 144, "jolt_js.go cWheelDesc");
static_assert(sizeof(ILL_VehicleDesc) == 128, "jolt_js.go cVehicleDesc");
static_assert(sizeof(ILL_Differential) == 24, "jolt_js.go cDifferential");
static_assert(sizeof(ILL_AntiRollBar) == 12, "jolt_js.go cAntiRollBar");
#endif

extern "C" {

void ILL_Init(void) {
	static std::once_flag once;
	std::call_once(once, [] {
		RegisterDefaultAllocator();
		Factory::sInstance = new Factory();
		RegisterTypes();
	});
}

ILL_World* ILL_World_New(uint32_t maxBodies) {
	ILL_Init();
	ILL_World* w = new ILL_World();
	w->system.Init(maxBodies, 0, std::max<uint32_t>(maxBodies, 1024) * 4, std::max<uint32_t>(maxBodies, 1024) * 2,
		w->bpLayers, w->objectVsBP, w->pairs);
	w->system.SetContactListener(&w->contacts);
	return w;
}

void ILL_World_Delete(ILL_World* w) {
	while (!w->vehicles.empty()) {
		w->removeVehicle(w->vehicles.begin()->first);
	}
	delete w;
}

int ILL_World_Step(ILL_World* w, float dt, int collisionSteps) {
	int err = int(w->system.Update(dt, collisionSteps, &w->temp, &w->jobs));
	w->contacts.Resolve(w->system.GetBodyInterface());
	return err;
}

void ILL_World_SetGravity(ILL_World* w, const float g[3]) { w->system.SetGravity(vec3(g)); }
void ILL_World_GetGravity(ILL_World* w, float out[3]) { put(w->system.GetGravity(), out); }

ILL_Shape* ILL_Shape_Box(const float h[3]) {
	float smallest = std::min(h[0], std::min(h[1], h[2]));
	return keep(BoxShapeSettings(vec3(h), convexRadius(smallest)).Create());
}

ILL_Shape* ILL_Shape_Sphere(float radius) { return keep(SphereShapeSettings(radius).Create()); }

ILL_Shape* ILL_Shape_Capsule(float halfHeight, float radius) {
	return keep(CapsuleShapeSettings(halfHeight, radius).Create());
}

ILL_Shape* ILL_Shape_Cylinder(float halfHeight, float radius) {
	return keep(CylinderShapeSettings(halfHeight, radius, convexRadius(std::min(halfHeight, radius))).Create());
}

ILL_Shape* ILL_Shape_ConvexHull(const float* points, int n) {
	Array<Vec3> pts;
	pts.reserve(n);
	for (int i = 0; i < n; i++) {
		pts.push_back(vec3(points + 3 * i));
	}
	return keep(ConvexHullShapeSettings(pts).Create());
}

ILL_Shape* ILL_Shape_Mesh(const float* vertices, int nv, const uint32_t* indices, int ntri) {
	VertexList verts;
	verts.reserve(nv);
	for (int i = 0; i < nv; i++) {
		verts.push_back(Float3(vertices[3 * i], vertices[3 * i + 1], vertices[3 * i + 2]));
	}
	IndexedTriangleList tris;
	tris.reserve(ntri);
	for (int i = 0; i < ntri; i++) {
		tris.push_back(IndexedTriangle(indices[3 * i], indices[3 * i + 1], indices[3 * i + 2]));
	}
	return keep(MeshShapeSettings(verts, tris).Create());
}

ILL_Shape* ILL_Shape_Offset(ILL_Shape* inner, const float offset[3]) {
	return keep(RotatedTranslatedShapeSettings(vec3(offset), Quat::sIdentity(), shapeOf(inner)).Create());
}

void ILL_Shape_Release(ILL_Shape* s) {
	if (s) {
		shapeOf(s)->Release();
	}
}

void ILL_BodySettings_Default(ILL_BodySettings* s) {
	*s = ILL_BodySettings{};
	s->transform[6] = 1;
	s->motion = ILL_DYNAMIC;
	s->allowSleeping = 1;
	s->allowedDOFs = uint32_t(EAllowedDOFs::All);
	s->friction = 0.5f;
	s->linearDamping = 0.05f;
	s->angularDamping = 0.05f;
	s->gravityFactor = 1;
}

ILL_BodyID ILL_Body_Create(ILL_World* w, const ILL_BodySettings* s) {
	EMotionType motion = s->motion == ILL_STATIC ? EMotionType::Static
		: s->motion == ILL_KINEMATIC            ? EMotionType::Kinematic
		                                        : EMotionType::Dynamic;
	// Static sensors are made kinematic so they detect static and kinematic
	// bodies too; dynamic sensors stay dynamic and fall like any other body.
	if (s->sensor && motion == EMotionType::Static) {
		motion = EMotionType::Kinematic;
	}
	ObjectLayer layer = motion == EMotionType::Static ? kNonMoving : s->character ? kCharacter : kMoving;

	BodyCreationSettings bs(shapeOf(s->shape), rvec3(s->transform), quat(s->transform + 3), motion, layer);
	bs.mLinearVelocity = vec3(s->linearVelocity);
	bs.mAngularVelocity = vec3(s->angularVelocity);
	bs.mIsSensor = s->sensor != 0;
	bs.mCollideKinematicVsNonDynamic = s->sensor != 0;
	bs.mAllowSleeping = s->allowSleeping != 0 && !s->sensor;
	bs.mMotionQuality = s->continuous ? EMotionQuality::LinearCast : EMotionQuality::Discrete;
	bs.mAllowedDOFs = EAllowedDOFs(s->allowedDOFs & uint32_t(EAllowedDOFs::All));
	bs.mFriction = s->friction;
	bs.mRestitution = s->restitution;
	bs.mLinearDamping = s->linearDamping;
	bs.mAngularDamping = s->angularDamping;
	bs.mGravityFactor = s->gravityFactor;
	bs.mUserData = s->userData;
	if (s->mass > 0) {
		bs.mOverrideMassProperties = EOverrideMassProperties::CalculateInertia;
		bs.mMassPropertiesOverride.mMass = s->mass;
	}
	BodyID id = w->system.GetBodyInterface().CreateAndAddBody(bs,
		motion == EMotionType::Static ? EActivation::DontActivate : EActivation::Activate);
	return id.GetIndexAndSequenceNumber();
}

void ILL_Body_Destroy(ILL_World* w, ILL_BodyID raw) {
	BodyInterface& bi = w->system.GetBodyInterface();
	BodyID id(raw);
	w->removeVehicle(raw);
	w->contacts.EndAll(raw);
	if (bi.IsAdded(id)) {
		bi.RemoveBody(id);
	}
	bi.DestroyBody(id);
}

void ILL_Body_GetTransform(ILL_World* w, ILL_BodyID id, float out[7]) {
	RVec3 p;
	Quat q;
	w->system.GetBodyInterface().GetPositionAndRotation(BodyID(id), p, q);
	putR(p, out);
	putQ(q, out + 3);
}

void ILL_Body_SetTransform(ILL_World* w, ILL_BodyID id, const float t[7], int activate) {
	w->system.GetBodyInterface().SetPositionAndRotation(BodyID(id), rvec3(t), quat(t + 3),
		activate ? EActivation::Activate : EActivation::DontActivate);
}

void ILL_Body_MoveKinematic(ILL_World* w, ILL_BodyID id, const float t[7], float dt) {
	w->system.GetBodyInterface().MoveKinematic(BodyID(id), rvec3(t), quat(t + 3), dt);
}

void ILL_Body_GetVelocity(ILL_World* w, ILL_BodyID id, float out[6]) {
	Vec3 lin, ang;
	w->system.GetBodyInterface().GetLinearAndAngularVelocity(BodyID(id), lin, ang);
	put(lin, out);
	put(ang, out + 3);
}

void ILL_Body_SetVelocity(ILL_World* w, ILL_BodyID id, const float v[6]) {
	BodyInterface& bi = w->system.GetBodyInterface();
	bi.SetLinearAndAngularVelocity(BodyID(id), vec3(v), vec3(v + 3));
	bi.ActivateBody(BodyID(id));
}

void ILL_Body_AddForce(ILL_World* w, ILL_BodyID id, const float f[3]) {
	w->system.GetBodyInterface().AddForce(BodyID(id), vec3(f));
}

void ILL_Body_AddTorque(ILL_World* w, ILL_BodyID id, const float t[3]) {
	w->system.GetBodyInterface().AddTorque(BodyID(id), vec3(t));
}

void ILL_Body_AddImpulse(ILL_World* w, ILL_BodyID id, const float i[3]) {
	w->system.GetBodyInterface().AddImpulse(BodyID(id), vec3(i));
}

void ILL_Body_AddAngularImpulse(ILL_World* w, ILL_BodyID id, const float i[3]) {
	w->system.GetBodyInterface().AddAngularImpulse(BodyID(id), vec3(i));
}

int ILL_Body_IsActive(ILL_World* w, ILL_BodyID id) { return w->system.GetBodyInterface().IsActive(BodyID(id)); }
void ILL_Body_Activate(ILL_World* w, ILL_BodyID id) { w->system.GetBodyInterface().ActivateBody(BodyID(id)); }

int ILL_World_DrainContacts(ILL_World* w, ILL_ContactEvent* out, int cap) { return w->contacts.Drain(out, cap); }

int ILL_World_CastRay(ILL_World* w, const float origin[3], const float dir[3], ILL_BodyID ignore, ILL_RayHit* out) {
	RRayCast ray(rvec3(origin), vec3(dir));
	RayCastResult hit;
	IgnoreSingleBodyFilter ignoreOne{BodyID(ignore)};
	BodyFilter everything;
	const BodyFilter& filter = ignore == ILL_INVALID_BODY ? everything : static_cast<const BodyFilter&>(ignoreOne);
	if (!w->system.GetNarrowPhaseQuery().CastRay(ray, hit, {}, {}, filter)) {
		return 0;
	}
	RVec3 point = ray.GetPointOnRay(hit.mFraction);
	Vec3 normal = -ray.mDirection.NormalizedOr(Vec3::sAxisY());
	BodyLockRead lock(w->system.GetBodyLockInterface(), hit.mBodyID);
	if (lock.Succeeded()) {
		normal = lock.GetBody().GetWorldSpaceSurfaceNormal(hit.mSubShapeID2, point);
	}
	out->body = hit.mBodyID.GetIndexAndSequenceNumber();
	out->fraction = hit.mFraction;
	putR(point, out->point);
	put(normal, out->normal);
	return 1;
}

// Clearance queries ignore trigger volumes as well as the requesting character.
class CapsuleBodyFilter : public IgnoreSingleBodyFilter {
public:
 using IgnoreSingleBodyFilter::IgnoreSingleBodyFilter;
 bool ShouldCollideLocked(const Body& body) const override { return !body.IsSensor(); }
};

int ILL_World_OverlapCapsule(ILL_World* w, const float center[3], float radius, float height, ILL_BodyID ignore) {
 CapsuleShape shape(std::max(0.001f, height / 2 - radius), radius);
 CapsuleBodyFilter filter{BodyID(ignore)};
 AllHitCollisionCollector<CollideShapeCollector> hits;
 w->system.GetNarrowPhaseQuery().CollideShape(&shape, Vec3::sReplicate(1), RMat44::sTranslation(rvec3(center)), {}, RVec3::sZero(), hits, {}, {}, filter);
 for (const auto& h : hits.mHits) if (h.mPenetrationDepth > 0.005f) return 1;
 return 0;
}

int ILL_World_SweepCapsule(ILL_World* w, const float center[3], const float delta[3], float radius, float height, ILL_BodyID ignore, ILL_RayHit* out) {
 CapsuleShape shape(std::max(0.001f, height / 2 - radius), radius);
 CapsuleBodyFilter filter{BodyID(ignore)};
 AllHitCollisionCollector<CastShapeCollector> hits;
 RShapeCast cast(&shape, Vec3::sReplicate(1), RMat44::sTranslation(rvec3(center)), vec3(delta));
 w->system.GetNarrowPhaseQuery().CastShape(cast, {}, RVec3::sZero(), hits, {}, {}, filter);
 const ShapeCastResult* best = nullptr;
 for (const auto& h : hits.mHits) {
  Vec3 normal = -h.mPenetrationAxis.NormalizedOr(Vec3::sAxisY());
  // Ignore touching support surfaces when moving parallel to or away from them.
  if (h.mPenetrationDepth <= 0.005f && normal.Dot(vec3(delta)) >= -0.0001f) continue;
  if (!best || h.mFraction < best->mFraction) best = &h;
 }
 if (!best) return 0;
 out->body = best->mBodyID2.GetIndexAndSequenceNumber();
 out->fraction = best->mFraction;
 put(best->mContactPointOn2, out->point);
 put(-best->mPenetrationAxis.NormalizedOr(Vec3::sAxisY()), out->normal);
 return 1;
}

void ILL_Character_UpdateControlled(ILL_Character* c, float dt) {
 CharacterVirtual::ExtendedUpdateSettings settings;
 settings.mWalkStairsStepUp = Vec3::sZero();
 settings.mStickToFloorStepDown = Vec3::sZero();
 PhysicsSystem& sys = c->world->system;
 c->ch->ExtendedUpdate(dt, Vec3::sZero(), settings, sys.GetDefaultBroadPhaseLayerFilter(kMoving), sys.GetDefaultLayerFilter(kMoving), {}, {}, c->world->temp);
}

ILL_Character* ILL_Character_New(ILL_World* w, ILL_Shape* shape, const float position[3], float maxSlope, float supportRadius) {
	CharacterVirtualSettings s;
	s.mShape = shapeOf(shape);
	s.mMaxSlopeAngle = maxSlope;
	s.mUp = Vec3::sAxisY();
	// Only contacts on the bottom sphere of the capsule count as ground.
	s.mSupportingVolume = Plane(Vec3::sAxisY(), -supportRadius);
	// An inner rigid body lets sensors and other bodies see the character.
	s.mInnerBodyShape = shapeOf(shape);
	s.mInnerBodyLayer = kCharacter;
	ILL_Character* c = new ILL_Character{w, nullptr};
	c->ch = new CharacterVirtual(&s, rvec3(position), Quat::sIdentity(), 0, &w->system);
	c->ch->SetListener(&w->characterContacts);
	return c;
}

void ILL_Character_Delete(ILL_Character* c) {
	c->world->contacts.EndAll(c->ch->GetInnerBodyID().GetIndexAndSequenceNumber());
	delete c;
}

void ILL_Character_Update(ILL_Character* c, float dt, float stepUp) {
	CharacterVirtual::ExtendedUpdateSettings settings;
	settings.mWalkStairsStepUp = Vec3(0, stepUp, 0);
	PhysicsSystem& sys = c->world->system;
	c->ch->ExtendedUpdate(dt, sys.GetGravity(), settings,
		sys.GetDefaultBroadPhaseLayerFilter(kMoving), sys.GetDefaultLayerFilter(kMoving),
		{}, {}, c->world->temp);
}

void ILL_Character_GetPosition(ILL_Character* c, float out[3]) { putR(c->ch->GetPosition(), out); }
void ILL_Character_SetPosition(ILL_Character* c, const float p[3]) { c->ch->SetPosition(rvec3(p)); }
void ILL_Character_GetVelocity(ILL_Character* c, float out[3]) { put(c->ch->GetLinearVelocity(), out); }
void ILL_Character_SetVelocity(ILL_Character* c, const float v[3]) { c->ch->SetLinearVelocity(vec3(v)); }
void ILL_Character_GetGroundVelocity(ILL_Character* c, float out[3]) { put(c->ch->GetGroundVelocity(), out); }
void ILL_Character_GetGroundNormal(ILL_Character* c, float out[3]) { put(c->ch->GetGroundNormal(), out); }
int ILL_Character_IsSupported(ILL_Character* c) { return c->ch->IsSupported(); }
ILL_BodyID ILL_Character_InnerBody(ILL_Character* c) { return c->ch->GetInnerBodyID().GetIndexAndSequenceNumber(); }

ILL_Shape* ILL_Shape_OffsetCenterOfMass(ILL_Shape* inner, const float offset[3]) {
	return keep(OffsetCenterOfMassShapeSettings(vec3(offset), shapeOf(inner)).Create());
}

void ILL_Shape_GetCenterOfMass(ILL_Shape* s, float out[3]) { put(shapeOf(s)->GetCenterOfMass(), out); }

void ILL_WheelDesc_Default(ILL_WheelDesc* out) {
	WheelSettingsWV d;
	*out = ILL_WheelDesc{};
	put(d.mPosition, out->position);
	put(d.mSuspensionDirection, out->suspensionDir);
	put(d.mSteeringAxis, out->steeringAxis);
	put(d.mWheelUp, out->wheelUp);
	put(d.mWheelForward, out->wheelForward);
	// A model built in the body's frame: right is forward × up.
	put(d.mWheelForward.Cross(d.mWheelUp), out->modelRight);
	put(d.mWheelUp, out->modelUp);
	out->radius = d.mRadius;
	out->width = d.mWidth;
	out->suspensionMin = d.mSuspensionMinLength;
	out->suspensionMax = d.mSuspensionMaxLength;
	out->preload = d.mSuspensionPreloadLength;
	out->frequency = d.mSuspensionSpring.mFrequency;
	out->damping = d.mSuspensionSpring.mDamping;
	out->maxSteer = d.mMaxSteerAngle;
	out->maxBrakeTorque = d.mMaxBrakeTorque;
	out->maxHandBrakeTorque = d.mMaxHandBrakeTorque;
	out->inertia = d.mInertia;
	out->angularDamping = d.mAngularDamping;
	out->longitudinalGrip = 1;
	out->lateralGrip = 1;
}

void ILL_VehicleDesc_Default(ILL_VehicleDesc* out) {
	VehicleConstraintSettings c;
	MotorcycleControllerSettings m; // a WheeledVehicleControllerSettings, plus lean
	*out = ILL_VehicleDesc{};
	put(c.mUp, out->up);
	put(c.mForward, out->forward);
	out->maxPitchRoll = c.mMaxPitchRollAngle;
	out->controller = ILL_WHEELED;
	out->tester = ILL_TEST_CYLINDER;
	out->maxTorque = m.mEngine.mMaxTorque;
	out->minRPM = m.mEngine.mMinRPM;
	out->maxRPM = m.mEngine.mMaxRPM;
	out->engineInertia = m.mEngine.mInertia;
	out->engineDamping = m.mEngine.mAngularDamping;
	out->numGears = int32_t(std::min<size_t>(8, m.mTransmission.mGearRatios.size()));
	for (int i = 0; i < out->numGears; i++) {
		out->gears[i] = m.mTransmission.mGearRatios[i];
	}
	out->reverseGear = m.mTransmission.mReverseGearRatios.empty() ? -2.9f : m.mTransmission.mReverseGearRatios[0];
	out->shiftUpRPM = m.mTransmission.mShiftUpRPM;
	out->shiftDownRPM = m.mTransmission.mShiftDownRPM;
	out->clutchStrength = m.mTransmission.mClutchStrength;
	out->switchTime = m.mTransmission.mSwitchTime;
	out->limitedSlipRatio = m.mDifferentialLimitedSlipRatio;
	out->maxLean = m.mMaxLeanAngle;
	out->leanSpring = m.mLeanSpringConstant;
	out->leanDamping = m.mLeanSpringDamping;
}

void ILL_Differential_Default(ILL_Differential* out) {
	VehicleDifferentialSettings d;
	out->left = d.mLeftWheel;
	out->right = d.mRightWheel;
	out->ratio = d.mDifferentialRatio;
	out->split = d.mLeftRightSplit;
	out->torqueRatio = d.mEngineTorqueRatio;
	out->limitedSlip = d.mLimitedSlipRatio;
}

int ILL_Vehicle_Create(ILL_World* w, ILL_BodyID raw, const ILL_VehicleDesc* d, const ILL_WheelDesc* wheels,
	int numWheels, const ILL_Differential* diffs, int numDiffs, const ILL_AntiRollBar* bars, int numBars) {
	if (w->vehicles.count(raw) || numWheels <= 0) {
		return 0;
	}

	VehicleConstraintSettings vs;
	vs.mUp = vec3(d->up).NormalizedOr(Vec3::sAxisY());
	vs.mForward = vec3(d->forward).NormalizedOr(Vec3::sAxisZ());
	vs.mMaxPitchRollAngle = d->maxPitchRoll;

	ILL_World::Vehicle v;
	for (int i = 0; i < numWheels; i++) {
		const ILL_WheelDesc& s = wheels[i];
		WheelSettingsWV* ws = new WheelSettingsWV;
		ws->mPosition = vec3(s.position);
		ws->mSuspensionDirection = vec3(s.suspensionDir).NormalizedOr(-vs.mUp);
		ws->mSteeringAxis = vec3(s.steeringAxis).NormalizedOr(vs.mUp);
		ws->mWheelUp = vec3(s.wheelUp).NormalizedOr(vs.mUp);
		ws->mWheelForward = vec3(s.wheelForward).NormalizedOr(vs.mForward);
		ws->mRadius = s.radius;
		ws->mWidth = s.width;
		ws->mSuspensionMinLength = s.suspensionMin;
		ws->mSuspensionMaxLength = std::max(s.suspensionMin, s.suspensionMax);
		ws->mSuspensionPreloadLength = s.preload;
		ws->mSuspensionSpring = SpringSettings(
			s.stiffness ? ESpringMode::StiffnessAndDamping : ESpringMode::FrequencyAndDamping, s.frequency, s.damping);
		ws->mMaxSteerAngle = s.maxSteer;
		ws->mMaxBrakeTorque = s.maxBrakeTorque;
		ws->mMaxHandBrakeTorque = s.maxHandBrakeTorque;
		ws->mInertia = s.inertia;
		ws->mAngularDamping = s.angularDamping;
		ws->mLongitudinalFriction = scaled(ws->mLongitudinalFriction, s.longitudinalGrip);
		ws->mLateralFriction = scaled(ws->mLateralFriction, s.lateralGrip);
		vs.mWheels.push_back(ws);
		v.right.push_back(vec3(s.modelRight).NormalizedOr(-Vec3::sAxisX()));
		v.up.push_back(vec3(s.modelUp).NormalizedOr(Vec3::sAxisY()));
	}

	WheeledVehicleControllerSettings* cs;
	if (d->controller == ILL_MOTORCYCLE) {
		MotorcycleControllerSettings* m = new MotorcycleControllerSettings;
		m->mMaxLeanAngle = d->maxLean;
		m->mLeanSpringConstant = d->leanSpring;
		m->mLeanSpringDamping = d->leanDamping;
		cs = m;
	} else {
		cs = new WheeledVehicleControllerSettings;
	}
	cs->mEngine.mMaxTorque = d->maxTorque;
	cs->mEngine.mMinRPM = d->minRPM;
	cs->mEngine.mMaxRPM = d->maxRPM;
	cs->mEngine.mInertia = d->engineInertia;
	cs->mEngine.mAngularDamping = d->engineDamping;
	cs->mTransmission.mGearRatios.clear();
	for (int i = 0; i < std::min(8, int(d->numGears)); i++) {
		cs->mTransmission.mGearRatios.push_back(d->gears[i]);
	}
	if (cs->mTransmission.mGearRatios.empty()) {
		cs->mTransmission.mGearRatios.push_back(1);
	}
	cs->mTransmission.mReverseGearRatios = {d->reverseGear < 0 ? d->reverseGear : -d->reverseGear};
	cs->mTransmission.mShiftUpRPM = d->shiftUpRPM;
	cs->mTransmission.mShiftDownRPM = d->shiftDownRPM;
	cs->mTransmission.mClutchStrength = d->clutchStrength;
	cs->mTransmission.mSwitchTime = d->switchTime;
	cs->mDifferentialLimitedSlipRatio = d->limitedSlipRatio;
	for (int i = 0; i < numDiffs; i++) {
		VehicleDifferentialSettings ds;
		ds.mLeftWheel = diffs[i].left < numWheels ? diffs[i].left : -1;
		ds.mRightWheel = diffs[i].right < numWheels ? diffs[i].right : -1;
		ds.mDifferentialRatio = diffs[i].ratio;
		ds.mLeftRightSplit = diffs[i].split;
		ds.mEngineTorqueRatio = diffs[i].torqueRatio;
		ds.mLimitedSlipRatio = diffs[i].limitedSlip;
		cs->mDifferentials.push_back(ds);
	}
	vs.mController = cs;
	for (int i = 0; i < numBars; i++) {
		if (bars[i].left < 0 || bars[i].left >= numWheels || bars[i].right < 0 || bars[i].right >= numWheels) {
			continue;
		}
		VehicleAntiRollBar bar;
		bar.mLeftWheel = bars[i].left;
		bar.mRightWheel = bars[i].right;
		bar.mStiffness = bars[i].stiffness;
		vs.mAntiRollBars.push_back(bar);
	}

	BodyLockWrite lock(w->system.GetBodyLockInterface(), BodyID(raw));
	if (!lock.Succeeded() || !lock.GetBody().IsDynamic()) {
		return 0;
	}
	v.constraint = new VehicleConstraint(lock.GetBody(), vs);
	VehicleCollisionTester* tester = nullptr;
	switch (d->tester) {
	case ILL_TEST_RAY:
		tester = new VehicleCollisionTesterRay(kMoving, vs.mUp);
		break;
	case ILL_TEST_SPHERE: {
		float r = FLT_MAX;
		for (int i = 0; i < numWheels; i++) {
			r = std::min(r, 0.5f * wheels[i].width);
		}
		tester = new VehicleCollisionTesterCastSphere(kMoving, r, vs.mUp);
		break;
	}
	default:
		tester = new VehicleCollisionTesterCastCylinder(kMoving);
	}
	tester->SetObjectLayerFilter(&kWheelLayers);
	v.constraint->SetVehicleCollisionTester(tester);
	lock.ReleaseLock();
	w->system.GetBodyInterface().SetObjectLayer(BodyID(raw), kVehicle);
	w->system.AddConstraint(v.constraint);
	w->system.AddStepListener(v.constraint);
	w->vehicles.emplace(raw, std::move(v));
	return 1;
}

void ILL_Vehicle_Destroy(ILL_World* w, ILL_BodyID body) { w->removeVehicle(body); }

void ILL_Vehicle_SetInput(ILL_World* w, ILL_BodyID body, float forward, float right, float brake, float handBrake) {
	auto it = w->vehicles.find(body);
	if (it == w->vehicles.end()) {
		return;
	}
	auto* c = static_cast<WheeledVehicleController*>(it->second.constraint->GetController());
	c->SetDriverInput(forward, right, brake, handBrake);
	if (forward != 0 || right != 0 || brake != 0 || handBrake != 0) {
		w->system.GetBodyInterface().ActivateBody(BodyID(body));
	}
}

int ILL_Vehicle_GetWheels(ILL_World* w, ILL_BodyID body, float* out, int cap) {
	auto it = w->vehicles.find(body);
	if (it == w->vehicles.end()) {
		return 0;
	}
	const ILL_World::Vehicle& v = it->second;
	const VehicleConstraint& c = *v.constraint;
	int n = int(c.GetWheels().size());
	for (int i = 0; i < std::min(n, cap); i++) {
		const Wheel* wheel = c.GetWheel(i);
		Mat44 m = c.GetWheelLocalTransform(i, v.right[i], v.up[i]);
		float* o = out + ILL_WHEEL_STATE * i;
		put(m.GetTranslation(), o);
		putQ(m.GetQuaternion(), o + 3);
		o[7] = wheel->GetAngularVelocity();
		o[8] = wheel->GetSteerAngle();
		o[9] = wheel->GetSuspensionLength();
		o[10] = wheel->HasContact() ? 1.0f : 0.0f;
	}
	return n;
}

void ILL_Vehicle_GetStatus(ILL_World* w, ILL_BodyID body, float out[4]) {
	out[0] = out[1] = out[2] = out[3] = 0;
	auto it = w->vehicles.find(body);
	if (it == w->vehicles.end()) {
		return;
	}
	const VehicleConstraint& c = *it->second.constraint;
	auto* ctrl = static_cast<const WheeledVehicleController*>(c.GetController());
	out[0] = ctrl->GetEngine().GetCurrentRPM();
	out[1] = float(ctrl->GetTransmission().GetCurrentGear());
	BodyInterface& bi = w->system.GetBodyInterface();
	Vec3 forward = bi.GetRotation(BodyID(body)) * c.GetLocalForward();
	out[2] = bi.GetLinearVelocity(BodyID(body)).Dot(forward);
	int touching = 0;
	for (const Wheel* wheel : c.GetWheels()) {
		touching += wheel->HasContact() ? 1 : 0;
	}
	out[3] = float(touching);
}

} // extern "C"
