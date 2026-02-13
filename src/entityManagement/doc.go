// Package entityManagement provides a generic CRUD framework for database entities
// with automatic route generation, Swagger documentation, and hook system integration.
//
// # Architecture
//
// The entity management system consists of four layers:
//
//   - Controllers (routes/): HTTP request handling and routing
//   - Services (services/): Business logic, hooks, and orchestration
//   - Repositories (repositories/): Database operations using GORM
//   - Models (models/): Base entity definitions
//
// # Usage
//
// To create a new managed entity:
//
// 1. Define your model:
//
//	type Course struct {
//	    models.BaseModel
//	    Title  string    `json:"title"`
//	    Code   string    `json:"code"`
//	}
//
// 2. Define DTOs and converters, then register using RegisterTypedEntity:
//
//	ems.RegisterTypedEntity[models.Course, dto.CourseInput, dto.CourseEdit, dto.CourseOutput](
//	    ems.GlobalEntityRegistrationService,
//	    "Course",
//	    entityManagementInterfaces.TypedEntityRegistration[models.Course, dto.CourseInput, dto.CourseEdit, dto.CourseOutput]{
//	        Converters: entityManagementInterfaces.TypedEntityConverters[models.Course, dto.CourseInput, dto.CourseEdit, dto.CourseOutput]{
//	            ModelToDto: dto.CourseModelToOutput,
//	            DtoToModel: dto.CourseInputToModel,
//	            DtoToMap:   dto.CourseEditToMap,
//	        },
//	        Roles: entityManagementInterfaces.EntityRoles{Roles: roleMap},
//	    },
//	)
//
// This automatically creates these routes:
//   - GET    /api/v1/courses           - List all courses (paginated)
//   - GET    /api/v1/courses/:id       - Get single course
//   - POST   /api/v1/courses           - Create course
//   - PATCH  /api/v1/courses/:id       - Update course
//   - DELETE /api/v1/courses/:id       - Delete course
//
// # Hooks
//
// Entities can execute code at lifecycle events:
//
//	hook := hooks.NewFunctionHook(
//	    "SendWelcomeEmail",
//	    "User",
//	    hooks.AfterCreate,
//	    func(ctx *hooks.HookContext) error {
//	        user := ctx.NewEntity.(*User)
//	        return sendEmail(user.Email, "Welcome!")
//	    },
//	)
//	hooks.GlobalHookRegistry.RegisterHook(hook)
//
// # Filtering
//
// Supports advanced filtering via query parameters:
//
//   - Direct fields: GET /courses?title=Golang
//   - Foreign keys: GET /chapters?courseId=123
//   - Many-to-many: GET /courses?tagIDs=1,2,3
//   - Nested relations: GET /pages?courseId=123 (via RelationshipFilter)
//
// # Permissions
//
// Integrates with Casbin for role-based access control. Permissions are
// automatically created when entities are registered.
//
// # Swagger Documentation
//
// API documentation is auto-generated from struct tags and registered
// entity metadata. Visit /swagger/ to view interactive docs.
package entityManagement
