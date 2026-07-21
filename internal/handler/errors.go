package handler

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/atiroop/pdlife/internal/mailer"
	"github.com/atiroop/pdlife/internal/models"
)

// A route's own handler already turns every failure it knows how to
// handle into an appropriate response — wrong password, 403, a JSON
// {"success":false} for the admin fetch() endpoints — by calling
// c.Render/c.JSON directly and returning nil. What lands here instead is
// only the case nothing anticipated: a DB call that errored, a template
// that failed to execute, a recovered panic, or Echo's own routing
// errors (404/405). Before this existed, all of that hit Echo's default
// HTTPErrorHandler, so a real breakage looked exactly like a bot probing
// /wp-admin in both the access log and to the user (Echo's bare default
// error page), and pdlife would only find out when a patient called.
//
// ErrorReporter renders a page that matches the rest of the app instead
// of Echo's default, and for anything actually server-side (5xx or a
// panic) emails admin — rate-limited, because the same code path failing
// on every request should produce one email to say "look at this", not
// one per request.
type ErrorReporter struct {
	mailer     *mailer.Mailer
	adminEmail string

	mu              sync.Mutex
	lastAlertAt     time.Time
	suppressedCount int
}

// alertCooldown is the minimum gap between server-error emails. Long
// enough that a bug hammering one endpoint doesn't flood the inbox; short
// enough that a genuinely new problem an hour later still gets its own
// alert promptly.
const alertCooldown = 15 * time.Minute

func NewErrorReporter(m *mailer.Mailer, adminEmail string) *ErrorReporter {
	return &ErrorReporter{mailer: m, adminEmail: adminEmail}
}

// HTTPErrorHandler replaces Echo's default (e.Echo.HTTPErrorHandler in
// main.go). Every error any handler returns, plus every panic Recover()
// catches (see RecoverLogErrorFunc below), ends up here exactly once.
func (r *ErrorReporter) HTTPErrorHandler(err error, c echo.Context) {
	code, message := httpErrorParts(err)

	// Recover's LogErrorFunc stashes the stack trace on the context
	// before handing the error to Echo, since a panic's stack is only
	// available at the moment it's recovered - by the time it reaches
	// here as a plain error, it's gone otherwise.
	stack, _ := c.Get(panicStackKey).(string)

	if code >= http.StatusInternalServerError {
		r.logAndAlert(err, code, message, stack, c)
	}

	if c.Response().Committed {
		return
	}
	r.respond(code, c)
}

func (r *ErrorReporter) logAndAlert(err error, code int, message, stack string, c echo.Context) {
	userID := ""
	if u, ok := c.Get(sessionUserKey).(*models.User); ok {
		userID = strconv.FormatUint(u.ID, 10)
	}

	// Structured, on one line, so it's greppable the same way as the
	// access log line echomw.Logger() already writes for this same
	// request - "status":5xx is the shared thread between them.
	log.Printf(`{"level":"error","status":%d,"method":%q,"path":%q,"error":%q,"user_id":%q}`,
		code, c.Request().Method, c.Request().URL.Path, err.Error(), userID)
	if stack != "" {
		log.Printf("panic stack trace:\n%s", stack)
	}

	if r.mailer == nil {
		return
	}

	r.mu.Lock()
	sinceLastAlert := time.Since(r.lastAlertAt)
	if !r.lastAlertAt.IsZero() && sinceLastAlert < alertCooldown {
		r.suppressedCount++
		r.mu.Unlock()
		return
	}
	suppressed := r.suppressedCount
	r.suppressedCount = 0
	r.lastAlertAt = time.Now()
	r.mu.Unlock()

	go func() {
		sendErr := r.mailer.SendServerErrorAlert(r.adminEmail, mailer.ServerErrorAlertData{
			Status:          code,
			Method:          c.Request().Method,
			Path:            c.Request().URL.Path,
			Error:           message,
			Stack:           stack,
			UserID:          userID,
			OccurredAt:      time.Now().Format("15:04:05 2 Jan 2006"),
			SuppressedCount: suppressed,
		})
		if sendErr != nil {
			log.Printf("error alert: failed to send: %v", sendErr)
		}
	}()
}

// respond sends the client something reasonable. Almost everything that
// reaches this point is HTML navigation (the JSON-returning endpoints -
// admin content-queue review, editorial save/upload - already handle
// their own failures as shown above and essentially never fall through
// to here); the X-Requested-With check covers /food-check's XHR results
// fragment, the one fetch() call in the app that does set it.
func (r *ErrorReporter) respond(code int, c echo.Context) {
	if c.Request().Header.Get(echo.HeaderXRequestedWith) == "XMLHttpRequest" {
		if jsonErr := c.JSON(code, map[string]interface{}{"success": false, "error": http.StatusText(code)}); jsonErr != nil {
			log.Printf("error handler: writing JSON error response failed: %v", jsonErr)
		}
		return
	}

	title, message := errorPageCopy(code)
	if renderErr := c.Render(code, "placeholder.html", map[string]string{"Title": title, "Message": message}); renderErr != nil {
		// The renderer itself failing (rather than the original handler
		// error) means something is wrong with the template pipeline -
		// don't try to render again and risk a loop, just degrade to plain
		// text so the request still ends.
		log.Printf("error handler: rendering error page failed: %v", renderErr)
		c.String(code, http.StatusText(code))
	}
}

func errorPageCopy(code int) (title, message string) {
	switch code {
	case http.StatusNotFound:
		return "ไม่พบหน้านี้", "ลิงก์นี้อาจถูกลบไปแล้ว หรือพิมพ์ที่อยู่ผิด"
	case http.StatusMethodNotAllowed:
		return "ไม่รองรับคำขอนี้", "กรุณากลับไปหน้าเดิมแล้วลองใหม่"
	case http.StatusBadRequest, http.StatusForbidden:
		// The realistic way a genuine patient hits this (as opposed to an
		// attacker, who doesn't need friendly copy): CSRF's token check
		// (main.go) rejects a form submitted from a page that sat open
		// across a deploy or a while - not a server problem, just a stale
		// page, and "reload" is the actual fix rather than "we'll look
		// into it".
		return "ทำรายการไม่สำเร็จ", "หน้านี้อาจค้างไว้นานเกินไป กรุณาโหลดหน้านี้ใหม่แล้วลองอีกครั้ง"
	default:
		return "เกิดข้อผิดพลาด", "ระบบขัดข้องชั่วคราว ทีมงานได้รับแจ้งแล้ว กรุณาลองใหม่อีกครั้งในอีกสักครู่"
	}
}

func httpErrorParts(err error) (code int, message string) {
	code = http.StatusInternalServerError
	message = err.Error()
	if he, ok := err.(*echo.HTTPError); ok {
		code = he.Code
		if s, ok := he.Message.(string); ok {
			message = s
		}
		if he.Internal != nil {
			message = fmt.Sprintf("%s (internal: %v)", message, he.Internal)
		}
	}
	return code, message
}

// panicStackKey is where RecoverLogErrorFunc stashes a panic's stack
// trace for HTTPErrorHandler to pick up - Recover's own callback is the
// only place that ever has it.
const panicStackKey = "pdlife_panic_stack"

// RecoverLogErrorFunc is passed to echomw.RecoverWithConfig's
// LogErrorFunc. Returning err (not nil) keeps Echo's normal behavior of
// continuing on to HTTPErrorHandler - a nil return would swallow the
// panic instead of reporting it.
func RecoverLogErrorFunc(c echo.Context, err error, stack []byte) error {
	c.Set(panicStackKey, string(stack))
	return err
}
