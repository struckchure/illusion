// A small C API over Jolt Physics, implemented in glue.cpp.
//
// Vectors cross the boundary as plain float arrays (never as aligned structs
// passed by value), which keeps cgo calls safe on every architecture.
// Transforms are float[7]: position xyz, then rotation quaternion xyzw.
#ifndef ILLUSION_JOLT_GLUE_H
#define ILLUSION_JOLT_GLUE_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct ILL_World ILL_World;
typedef struct ILL_Shape ILL_Shape;
typedef struct ILL_Character ILL_Character;
typedef uint32_t ILL_BodyID;

#define ILL_INVALID_BODY 0xffffffffu

enum { ILL_STATIC = 0, ILL_KINEMATIC = 1, ILL_DYNAMIC = 2 };
enum { ILL_CONTACT_BEGIN = 0, ILL_CONTACT_END = 1 };

// Global setup; safe to call more than once.
void ILL_Init(void);

ILL_World* ILL_World_New(uint32_t maxBodies);
void ILL_World_Delete(ILL_World* w);
int ILL_World_Step(ILL_World* w, float dt, int collisionSteps);
void ILL_World_SetGravity(ILL_World* w, const float g[3]);
void ILL_World_GetGravity(ILL_World* w, float out[3]);

// Shapes are returned holding one reference; release it with
// ILL_Shape_Release once bodies that use it have been created. NULL means the
// shape was invalid.
ILL_Shape* ILL_Shape_Box(const float halfExtents[3]);
ILL_Shape* ILL_Shape_Sphere(float radius);
ILL_Shape* ILL_Shape_Capsule(float halfHeightOfCylinder, float radius);
ILL_Shape* ILL_Shape_Cylinder(float halfHeight, float radius);
ILL_Shape* ILL_Shape_ConvexHull(const float* points, int numPoints);
ILL_Shape* ILL_Shape_Mesh(const float* vertices, int numVertices, const uint32_t* indices, int numTriangles);
// Offset returns a new shape: inner moved by offset. It does not consume inner.
ILL_Shape* ILL_Shape_Offset(ILL_Shape* inner, const float offset[3]);
void ILL_Shape_Release(ILL_Shape* s);

typedef struct ILL_BodySettings {
	ILL_Shape* shape;
	float transform[7];
	float linearVelocity[3];
	float angularVelocity[3];
	int motion;          // ILL_STATIC, ILL_KINEMATIC or ILL_DYNAMIC
	int sensor;          // detects overlaps without colliding
	int allowSleeping;
	int continuous;      // continuous collision detection for fast bodies
	uint32_t allowedDOFs; // bitmask: translation xyz = 1,2,4; rotation xyz = 8,16,32
	float friction;
	float restitution;
	float linearDamping;
	float angularDamping;
	float gravityFactor;
	float mass;          // <= 0: computed from the shape
	int character;       // character/ragdoll body: yields to vehicles
	uint64_t userData;
} ILL_BodySettings;

void ILL_BodySettings_Default(ILL_BodySettings* s);
ILL_BodyID ILL_Body_Create(ILL_World* w, const ILL_BodySettings* s);
void ILL_Body_Destroy(ILL_World* w, ILL_BodyID id);
void ILL_Body_GetTransform(ILL_World* w, ILL_BodyID id, float out[7]);
void ILL_Body_SetTransform(ILL_World* w, ILL_BodyID id, const float t[7], int activate);
void ILL_Body_MoveKinematic(ILL_World* w, ILL_BodyID id, const float t[7], float dt);
// Velocities are float[6]: linear xyz, then angular xyz.
void ILL_Body_GetVelocity(ILL_World* w, ILL_BodyID id, float out[6]);
void ILL_Body_SetVelocity(ILL_World* w, ILL_BodyID id, const float v[6]);
void ILL_Body_AddForce(ILL_World* w, ILL_BodyID id, const float f[3]);
void ILL_Body_AddTorque(ILL_World* w, ILL_BodyID id, const float t[3]);
void ILL_Body_AddImpulse(ILL_World* w, ILL_BodyID id, const float i[3]);
void ILL_Body_AddAngularImpulse(ILL_World* w, ILL_BodyID id, const float i[3]);
int ILL_Body_IsActive(ILL_World* w, ILL_BodyID id);
void ILL_Body_Activate(ILL_World* w, ILL_BodyID id);

typedef struct ILL_ContactEvent {
	ILL_BodyID body1;
	ILL_BodyID body2;
	int kind;        // ILL_CONTACT_BEGIN or ILL_CONTACT_END
	float point[3];  // world-space contact point (BEGIN only)
	float normal[3]; // from body1 toward body2 (BEGIN only)
} ILL_ContactEvent;

// Copies up to cap buffered contact events into out, oldest first, and
// returns how many were copied. Call until it returns less than cap.
int ILL_World_DrainContacts(ILL_World* w, ILL_ContactEvent* out, int cap);

typedef struct ILL_RayHit {
	ILL_BodyID body;
	float fraction;
	float point[3];
	float normal[3];
} ILL_RayHit;

// Casts a ray from origin along dir (whose length is the maximum distance),
// ignoring the body `ignore` (pass ILL_INVALID_BODY to ignore nothing).
int ILL_World_CastRay(ILL_World* w, const float origin[3], const float dir[3], ILL_BodyID ignore, ILL_RayHit* out);

int ILL_World_OverlapCapsule(ILL_World* w, const float center[3], float radius, float height, ILL_BodyID ignore);
int ILL_World_SweepCapsule(ILL_World* w, const float center[3], const float delta[3], float radius, float height, ILL_BodyID ignore, ILL_RayHit* out);
void ILL_Character_UpdateControlled(ILL_Character* c, float dt);

ILL_Character* ILL_Character_New(ILL_World* w, ILL_Shape* shape, const float position[3], float maxSlope, float supportRadius);
void ILL_Character_Delete(ILL_Character* c);
void ILL_Character_Update(ILL_Character* c, float dt, float stepUp);
void ILL_Character_GetPosition(ILL_Character* c, float out[3]);
void ILL_Character_SetPosition(ILL_Character* c, const float p[3]);
void ILL_Character_GetVelocity(ILL_Character* c, float out[3]);
void ILL_Character_SetVelocity(ILL_Character* c, const float v[3]);
void ILL_Character_GetGroundVelocity(ILL_Character* c, float out[3]);
void ILL_Character_GetGroundNormal(ILL_Character* c, float out[3]);
int ILL_Character_IsSupported(ILL_Character* c);
ILL_BodyID ILL_Character_InnerBody(ILL_Character* c);

// OffsetCenterOfMass returns a new shape: inner with its center of mass moved
// by offset. It does not consume inner.
ILL_Shape* ILL_Shape_OffsetCenterOfMass(ILL_Shape* inner, const float offset[3]);
// Writes the shape's center of mass, in its own space.
void ILL_Shape_GetCenterOfMass(ILL_Shape* s, float out[3]);

// Vehicles: Jolt's VehicleConstraint on a dynamic body, with wheels that cast
// against the world and are driven through an engine, a gearbox and
// differentials. A body has at most one vehicle, addressed by the body's ID
// and destroyed with it. Vectors are in the body's local space.
enum { ILL_WHEELED = 0, ILL_MOTORCYCLE = 1 };
enum { ILL_TEST_CYLINDER = 0, ILL_TEST_RAY = 1, ILL_TEST_SPHERE = 2 };

typedef struct ILL_WheelDesc {
	float position[3];      // where the suspension is attached
	float suspensionDir[3]; // pointing down
	float steeringAxis[3];  // pointing up
	float wheelUp[3];       // up at neutral steering
	float wheelForward[3];  // forward at neutral steering
	float modelRight[3];    // the wheel model's axis that read-back turns to face right
	float modelUp[3];       // the wheel model's axis that read-back turns to face up
	float radius;
	float width;
	float suspensionMin;    // suspension length fully raised
	float suspensionMax;    // suspension length fully drooped
	float preload;
	float frequency;        // suspension spring, Hz
	float damping;          // suspension spring, 0..1
	float maxSteer;         // radians; negative steers the other way
	float maxBrakeTorque;
	float maxHandBrakeTorque;
	float inertia;
	float angularDamping;
	float longitudinalGrip; // scales the tire's forward friction curve
	float lateralGrip;      // scales the tire's sideways friction curve
	int32_t stiffness;      // 1: frequency is the spring's stiffness (N/m), damping its N·s/m
} ILL_WheelDesc;

typedef struct ILL_VehicleDesc {
	float up[3];
	float forward[3];
	float maxPitchRoll; // radians; pi = no limit
	int32_t controller; // ILL_WHEELED or ILL_MOTORCYCLE
	int32_t tester;     // ILL_TEST_*
	float maxTorque;    // engine, Nm
	float minRPM;
	float maxRPM;
	float engineInertia;
	float engineDamping;
	float gears[8];     // forward gear ratios, first numGears used
	int32_t numGears;
	float reverseGear;  // negative
	float shiftUpRPM;
	float shiftDownRPM;
	float clutchStrength;
	float switchTime;
	float limitedSlipRatio; // between differentials
	float maxLean;          // motorcycle only, radians
	float leanSpring;
	float leanDamping;
} ILL_VehicleDesc;

typedef struct ILL_Differential {
	int32_t left, right; // wheel indices, -1 for none
	float ratio;
	float split;         // 0 = all to the left wheel, 1 = all to the right
	float torqueRatio;   // share of the engine's torque
	float limitedSlip;
} ILL_Differential;

typedef struct ILL_AntiRollBar {
	int32_t left, right;
	float stiffness;
} ILL_AntiRollBar;

void ILL_WheelDesc_Default(ILL_WheelDesc* out);
void ILL_VehicleDesc_Default(ILL_VehicleDesc* out);
void ILL_Differential_Default(ILL_Differential* out);
// Returns 0 if the body isn't a live dynamic body or already has a vehicle.
int ILL_Vehicle_Create(ILL_World* w, ILL_BodyID body, const ILL_VehicleDesc* desc, const ILL_WheelDesc* wheels,
	int numWheels, const ILL_Differential* diffs, int numDiffs, const ILL_AntiRollBar* bars, int numBars);
void ILL_Vehicle_Destroy(ILL_World* w, ILL_BodyID body);
// forward and right are -1..1, brake and handBrake 0..1. Any non-zero input
// wakes the body.
void ILL_Vehicle_SetInput(ILL_World* w, ILL_BodyID body, float forward, float right, float brake, float handBrake);
// Writes ILL_WHEEL_STATE floats per wheel, for up to cap wheels, and returns
// the vehicle's wheel count (0 without a vehicle): the wheel model's transform
// in the body's space (position xyz, rotation xyzw, with spin, steer and
// suspension), then its angular velocity (rad/s), steer angle, suspension
// length and 1 if it touches something.
#define ILL_WHEEL_STATE 11
int ILL_Vehicle_GetWheels(ILL_World* w, ILL_BodyID body, float* out, int cap);
// Writes the engine's rpm, the gear (-1 reverse, 0 neutral), the speed along
// the vehicle's forward (m/s) and how many wheels touch something.
void ILL_Vehicle_GetStatus(ILL_World* w, ILL_BodyID body, float out[4]);

#ifdef __cplusplus
}
#endif

#endif
