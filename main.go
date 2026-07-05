package main

import (
	"net/http"
	"os"

	"github.com/joho/godotenv"
	"github.com/labstack/echo/v4"
	echomw "github.com/labstack/echo/v4/middleware"
)

func main() {
	godotenv.Load()

	e := echo.New()
	e.HideBanner = true
	e.Use(echomw.Logger())
	e.Use(echomw.Recover())

	e.GET("/healthz", func(c echo.Context) error {
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	addr := os.Getenv("APP_ADDR")
	if addr == "" {
		// nginx on the production server proxies pdlife.app to this port
		addr = "127.0.0.1:8085"
	}
	e.Logger.Fatal(e.Start(addr))
}
