package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	fiberrecover "github.com/gofiber/fiber/v2/middleware/recover"

	"latihan-fiber/UTS/config"
	"latihan-fiber/UTS/database"
	"latihan-fiber/UTS/handler"
	"latihan-fiber/UTS/helper"
	"latihan-fiber/UTS/middleware"
	"latihan-fiber/UTS/migrations"
	"latihan-fiber/UTS/repository"
	"latihan-fiber/UTS/route"
	"latihan-fiber/UTS/seed"
	"latihan-fiber/UTS/service"
)

func main() {
	config.LoadEnv()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	pool, err := database.NewPool(ctx)
	cancel()
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()

	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	switch command {
	case "migrate":
		if err := migrations.Apply(context.Background(), pool); err != nil {
			log.Fatal(err)
		}
		log.Println("migration selesai")
	case "seed":
		if err := seed.Run(context.Background(), pool); err != nil {
			log.Fatal(err)
		}
		log.Println("seeder selesai")
	case "serve":
		jwtManager, err := helper.NewJWTManager(
			config.Get("JWT_SECRET", ""),
			time.Duration(config.GetInt("JWT_ACCESS_TTL_MINUTES", 15))*time.Minute,
		)
		if err != nil {
			log.Fatal(err)
		}
		authRepository := repository.NewAuthRepository(pool)
		authService := service.NewAuthService(authRepository, jwtManager)
		authHandler := handler.NewAuthHandler(authService)
		studentRepository := repository.NewStudentRepository(pool)
		studentService := service.NewStudentService(studentRepository)
		studentHandler := handler.NewStudentHandler(studentService)
		courseRepository := repository.NewCourseRepository(pool)
		courseService := service.NewCourseService(courseRepository)
		courseHandler := handler.NewCourseHandler(courseService)
		enrollmentRepository := repository.NewEnrollmentRepository(pool)
		enrollmentService := service.NewEnrollmentService(enrollmentRepository)
		enrollmentHandler := handler.NewEnrollmentHandler(enrollmentService)
		serve(authHandler, studentHandler, courseHandler, enrollmentHandler, jwtManager, authService)
	default:
		log.Fatalf("perintah tidak dikenal %q; gunakan serve, migrate, atau seed", command)
	}
}

func serve(authHandler *handler.AuthHandler, studentHandler *handler.StudentHandler, courseHandler *handler.CourseHandler, enrollmentHandler *handler.EnrollmentHandler, jwtManager *helper.JWTManager, authService middleware.IdentityAuthenticator) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	app := fiber.New(fiber.Config{AppName: "SIAKAD Mini", ErrorHandler: config.ErrorHandler})
	app.Use(fiberrecover.New())
	app.Use(middleware.RequestTracing(logger))
	route.Register(app, authHandler, studentHandler, courseHandler, enrollmentHandler, jwtManager, authService)

	port := config.Get("APP_PORT", "3000")
	go func() {
		if err := app.Listen(":" + port); err != nil {
			log.Printf("server berhenti: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := app.ShutdownWithContext(ctx); err != nil {
		log.Printf("shutdown server: %v", err)
	}
}
