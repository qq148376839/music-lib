package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// POST /api/login/qr/start?platform=netease|qq
func (s *Server) handleLoginQRStart(c *gin.Context) {
	platform := c.Query("platform")
	if !isValidPlatform(platform) {
		writeError(c, http.StatusBadRequest, "platform must be 'netease' or 'qq'")
		return
	}
	if err := s.loginMgr.StartLogin(platform); err != nil {
		writeError(c, http.StatusInternalServerError, err.Error())
		return
	}
	writeOK(c, map[string]string{"status": "started"})
}

// GET /api/login/qr/poll?platform=netease|qq
func (s *Server) handleLoginQRPoll(c *gin.Context) {
	platform := c.Query("platform")
	if !isValidPlatform(platform) {
		writeError(c, http.StatusBadRequest, "platform must be 'netease' or 'qq'")
		return
	}
	state, qrImage, nickname, errMsg := s.loginMgr.GetStatus(platform)
	writeOK(c, map[string]any{
		"state":    string(state),
		"qr_image": qrImage,
		"nickname": nickname,
		"error":    errMsg,
	})
}

// GET /api/login/status?platform=netease|qq
func (s *Server) handleLoginStatus(c *gin.Context) {
	platform := c.Query("platform")
	switch platform {
	case "netease":
		loggedIn, nickname := s.netease.GetLoginStatus()
		writeOK(c, map[string]any{
			"logged_in": loggedIn,
			"nickname":  nickname,
		})
	case "qq":
		loggedIn, nickname := s.qq.GetLoginStatus()
		writeOK(c, map[string]any{
			"logged_in": loggedIn,
			"nickname":  nickname,
		})
	default:
		writeError(c, http.StatusBadRequest, "platform must be 'netease' or 'qq'")
	}
}

// POST /api/login/cookie?platform=netease|qq
// Body: {"cookie": "...", "nickname": "..."}
func (s *Server) handleLoginCookie(c *gin.Context) {
	platform := c.Query("platform")
	if !isValidPlatform(platform) {
		writeError(c, http.StatusBadRequest, "platform must be 'netease' or 'qq'")
		return
	}
	var req struct {
		Cookie   string `json:"cookie"`
		Nickname string `json:"nickname"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Cookie == "" {
		writeError(c, http.StatusBadRequest, "cookie is required")
		return
	}
	s.loginMgr.SetCookie(platform, req.Cookie, req.Nickname)
	writeOK(c, map[string]string{"status": "ok"})
}

// POST /api/login/logout?platform=netease|qq
func (s *Server) handleLoginLogout(c *gin.Context) {
	platform := c.Query("platform")
	switch platform {
	case "netease":
		s.netease.Logout()
		writeOK(c, map[string]string{"status": "ok"})
	case "qq":
		s.qq.Logout()
		writeOK(c, map[string]string{"status": "ok"})
	default:
		writeError(c, http.StatusBadRequest, "platform must be 'netease' or 'qq'")
	}
}
