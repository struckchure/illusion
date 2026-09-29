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

#ifdef __cplusplus
}
#endif

#endif
