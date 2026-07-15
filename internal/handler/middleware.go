package handler

import (
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/models"
)

// The guards inside each handler (requireApdPatient, requireAdmin, ...)
// still do the fine-grained checks — verified email, completed onboarding,
// health-data consent, matching treatment type. What they cannot do is
// catch a route whose handler simply forgets to call one: that route is
// wide open, and nothing says so.
//
// These middlewares close that hole by moving the coarse check — is there a
// session at all, and is it an admin — onto the route group in main.go. A
// new route added to the group is gated whether or not its handler
// remembers to ask, and the guards keep doing the rest.
//
// They are cheap to stack with the in-handler guards because
// currentSession memoises per request: the middleware resolves the session,
// the handler's guard reads the same answer back out of the context.

// RequireSession rejects anyone without a valid session before the handler
// runs.
func (h *AuthHandler) RequireSession(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if _, err := h.currentSession(c); err != nil {
			return c.Redirect(http.StatusSeeOther, "/login")
		}
		return next(c)
	}
}

// RequireAdminRole rejects anyone who is not an Admin before the handler
// runs. It renders the same 403 as the in-handler requireAdmin guard, so
// which one fires first is not observable.
func (h *AuthHandler) RequireAdminRole(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		user, err := h.currentSession(c)
		if err != nil {
			return c.Redirect(http.StatusSeeOther, "/login")
		}
		if user.Role != models.RoleAdmin {
			return c.Render(http.StatusForbidden, "placeholder.html", map[string]string{
				"Title":   "ไม่มีสิทธิ์เข้าถึง",
				"Message": "หน้านี้ใช้ได้เฉพาะผู้ดูแลระบบ (Admin) เท่านั้น",
			})
		}
		return next(c)
	}
}
