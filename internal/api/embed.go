package api

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/guohuiyuan/music-lib/web"
)

// registerStaticFiles mounts the embedded web/dist/ directory as the SPA
// frontend. Non-file routes fallback to index.html for Vue Router history mode.
func registerStaticFiles(engine *gin.Engine) {
	sub, _ := fs.Sub(web.FS, "dist")
	fileServer := http.FileServer(http.FS(sub))

	engine.NoRoute(func(c *gin.Context) {
		p := strings.TrimPrefix(c.Request.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}

		// Serve the file if it exists in the embedded FS.
		if _, err := fs.Stat(sub, p); err == nil {
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		// SPA fallback: serve index.html for client-side routing.
		c.Request.URL.Path = "/"
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
}
