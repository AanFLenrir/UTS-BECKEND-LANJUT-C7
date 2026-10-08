package route

import (
	"github.com/gofiber/fiber/v2"

	"latihan-fiber/UTS/handler"
	"latihan-fiber/UTS/helper"
	"latihan-fiber/UTS/middleware"
)

func Register(app *fiber.App, authHandler *handler.AuthHandler, studentHandler *handler.StudentHandler, courseHandler *handler.CourseHandler, enrollmentHandler *handler.EnrollmentHandler, jwtManager *helper.JWTManager, authenticator middleware.IdentityAuthenticator) {
	api := app.Group("/api/v1")
	auth := api.Group("/auth")
	auth.Post("/login", middleware.LoginRateLimit(), authHandler.Login)
	auth.Get("/me", middleware.RequireAuth(jwtManager, authenticator), authHandler.Me)

	students := api.Group("/students", middleware.RequireAuth(jwtManager, authenticator))
	students.Get("/", middleware.RequireRole("admin"), studentHandler.List)
	students.Post("/", middleware.RequireRole("admin"), studentHandler.Create)
	students.Get("/:id", middleware.RequireRole("admin", "mahasiswa"), studentHandler.Get)
	students.Put("/:id", middleware.RequireRole("admin"), studentHandler.Update)
	students.Delete("/:id", middleware.RequireRole("admin"), studentHandler.Delete)

	courses := api.Group("/courses", middleware.RequireAuth(jwtManager, authenticator))
	courses.Get("/", middleware.RequireRole("admin", "mahasiswa"), courseHandler.List)

	enrollments := api.Group("/enrollments", middleware.RequireAuth(jwtManager, authenticator))
	enrollments.Post("/", middleware.RequireRole("mahasiswa"), enrollmentHandler.Create)
	enrollments.Delete("/:id", middleware.RequireRole("mahasiswa"), enrollmentHandler.Delete)
}
