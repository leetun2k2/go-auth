package transport

import (
	"github.com/gin-gonic/gin"
	"github.com/leetun2k2/go-auth/internal/handler"
)

func (app *Application) RegisterRestfulApi(engine *gin.Engine) error {
	engine.GET("/health", app.health)
	return nil
}

func (app *Application) health(c *gin.Context) {
	ctx := c.Request.Context()
	input := handler.HealthInput{}
	output, err := app.Handler.HealthHandler(ctx, &input)
	if err != nil {
		c.JSON(500, map[string]string{"error": err.Error()})
		return
	}
	c.JSON(output.StatusCode, output)
}
