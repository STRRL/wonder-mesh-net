package controller

import (
	"encoding/json"
	"errors"
	"html"
	"log/slog"
	"net/http"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/service"
)

// DeviceFlowController handles OAuth 2.0 Device Authorization Grant (RFC 8628).
type DeviceFlowController struct {
	deviceFlowService *service.DeviceFlowService
	authService       *service.AuthService
	publicURL         string
}

// NewDeviceFlowController creates a new DeviceFlowController.
func NewDeviceFlowController(
	deviceFlowService *service.DeviceFlowService,
	authService *service.AuthService,
	publicURL string,
) *DeviceFlowController {
	return &DeviceFlowController{
		deviceFlowService: deviceFlowService,
		authService:       authService,
		publicURL:         publicURL,
	}
}

// HandleDeviceCode handles POST /device/code requests.
func (c *DeviceFlowController) HandleDeviceCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	init, err := c.deviceFlowService.InitiateDeviceFlow(r.Context())
	if err != nil {
		slog.Error("create device request", "error", err)
		http.Error(w, "create device request", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(DeviceCodeResponse{
		DeviceCode:      init.DeviceCode,
		UserCode:        init.UserCode,
		VerificationURL: init.VerificationURL,
		ExpiresIn:       init.ExpiresIn,
		Interval:        init.Interval,
	}); err != nil {
		slog.Error("encode device code response", "error", err)
	}
}

// HandleDeviceVerifyPage handles GET /device/verify requests.
func (c *DeviceFlowController) HandleDeviceVerifyPage(w http.ResponseWriter, r *http.Request) {
	userCode := html.EscapeString(r.URL.Query().Get("code"))

	htmlContent := `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <title>Device Authorization - Wonder Mesh Net</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            padding: 20px;
        }
        .card {
            background: white;
            border-radius: 16px;
            padding: 40px;
            max-width: 400px;
            width: 100%;
            box-shadow: 0 20px 60px rgba(0,0,0,0.3);
        }
        h1 { font-size: 24px; margin-bottom: 8px; color: #1a1a2e; }
        .subtitle { color: #666; margin-bottom: 24px; }
        .code-input {
            width: 100%;
            padding: 16px;
            font-size: 24px;
            text-align: center;
            letter-spacing: 4px;
            border: 2px solid #e0e0e0;
            border-radius: 8px;
            margin-bottom: 16px;
            text-transform: uppercase;
        }
        .code-input:focus { outline: none; border-color: #667eea; }
        .btn {
            width: 100%;
            padding: 16px;
            font-size: 16px;
            font-weight: 600;
            color: white;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            border: none;
            border-radius: 8px;
            cursor: pointer;
            transition: transform 0.2s, box-shadow 0.2s;
        }
        .btn:hover { transform: translateY(-2px); box-shadow: 0 4px 12px rgba(102,126,234,0.4); }
        .btn:disabled { opacity: 0.6; cursor: not-allowed; transform: none; }
        .error { color: #e53935; margin-top: 16px; text-align: center; }
        .success {
            text-align: center;
            color: #43a047;
        }
        .success h2 { font-size: 48px; margin-bottom: 16px; }
        .login-prompt { margin-top: 24px; text-align: center; }
        .login-prompt a {
            color: #667eea;
            text-decoration: none;
            font-weight: 600;
        }
    </style>
</head>
<body>
    <div class="card" id="card">
        <div id="form-view">
            <h1>Device Authorization</h1>
            <p class="subtitle">Enter the code shown on your device</p>
            <form id="verify-form" method="POST">
                <input type="text" name="user_code" class="code-input"
                       placeholder="XXXX-XXXX" maxlength="9"
                       value="` + userCode + `" autocomplete="off" required>
                <button type="submit" class="btn" id="submit-btn">Authorize Device</button>
            </form>
            <div id="error" class="error"></div>
            <div class="login-prompt" id="login-prompt" style="display:none;">
                <a href="#" id="login-link">Login first to authorize this device</a>
            </div>
        </div>
        <div id="success-view" class="success" style="display:none;">
            <h2>✓</h2>
            <h1>Device Authorized!</h1>
            <p class="subtitle">You can close this window and return to your terminal.</p>
        </div>
    </div>
    <script>
        const form = document.getElementById('verify-form');
        const errorDiv = document.getElementById('error');
        const loginPrompt = document.getElementById('login-prompt');
        const formView = document.getElementById('form-view');
        const successView = document.getElementById('success-view');
        const submitBtn = document.getElementById('submit-btn');
        const codeInput = document.querySelector('.code-input');

        codeInput.addEventListener('input', function(e) {
            let v = e.target.value.replace(/[^A-Za-z0-9]/g, '').toUpperCase();
            if (v.length > 4) v = v.slice(0,4) + '-' + v.slice(4,8);
            e.target.value = v;
        });

        form.addEventListener('submit', async function(e) {
            e.preventDefault();
            errorDiv.textContent = '';
            loginPrompt.style.display = 'none';
            submitBtn.disabled = true;
            submitBtn.textContent = 'Authorizing...';

            try {
                const resp = await fetch('/coordinator/device/verify', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ user_code: codeInput.value }),
                    credentials: 'include'
                });
                const data = await resp.json();

                if (resp.ok) {
                    formView.style.display = 'none';
                    successView.style.display = 'block';
                } else if (resp.status === 401) {
                    loginPrompt.style.display = 'block';
                    document.getElementById('login-link').href =
                        '/coordinator/auth/login?provider=github&redirect=' +
                        encodeURIComponent(window.location.href);
                    errorDiv.textContent = data.error || 'Please login first';
                } else {
                    errorDiv.textContent = data.error || 'Verification failed';
                }
            } catch (err) {
                errorDiv.textContent = 'Network error, please try again';
            } finally {
                submitBtn.disabled = false;
                submitBtn.textContent = 'Authorize Device';
            }
        });
    </script>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write([]byte(htmlContent)); err != nil {
		slog.Error("write device verify page", "error", err)
	}
}

// HandleDeviceVerify handles POST /device/verify requests.
func (c *DeviceFlowController) HandleDeviceVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	realm, err := c.authService.GetRealmFromRequest(ctx, r)
	if err != nil || realm == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "login required"})
		return
	}

	var req struct {
		UserCode string `json:"user_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request"})
		return
	}

	if !c.deviceFlowService.ValidateUserCode(req.UserCode) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid code format, expected XXXX-XXXX"})
		return
	}

	err = c.deviceFlowService.ApproveDevice(ctx, req.UserCode, realm)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if errors.Is(err, repository.ErrDeviceRequestNotFound) {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid or expired code"})
		} else if errors.Is(err, service.ErrCodeAlreadyUsed) {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "code already used"})
		} else {
			slog.Error("approve device", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "approve device"})
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "approved"})
}

// HandleDeviceToken handles POST /device/token requests.
func (c *DeviceFlowController) HandleDeviceToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		DeviceCode string `json:"device_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(DeviceTokenResponse{Error: "invalid_request"})
		return
	}

	if !c.deviceFlowService.ValidateDeviceCode(req.DeviceCode) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(DeviceTokenResponse{Error: "invalid_device_code_format"})
		return
	}

	result, err := c.deviceFlowService.PollDeviceToken(r.Context(), req.DeviceCode)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if errors.Is(err, repository.ErrDeviceRequestNotFound) {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(DeviceTokenResponse{Error: "invalid_device_code"})
		} else {
			slog.Error("poll device token", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(DeviceTokenResponse{Error: "internal_error"})
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch result.Status {
	case "authorization_pending":
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(DeviceTokenResponse{Error: "authorization_pending"})
	case "approved":
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(DeviceTokenResponse{
			Authkey:      result.AuthKey,
			HeadscaleURL: result.HeadscaleURL,
			User:         result.User,
		})
	case "expired_token":
		w.WriteHeader(http.StatusGone)
		_ = json.NewEncoder(w).Encode(DeviceTokenResponse{Error: "expired_token"})
	case "access_denied":
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(DeviceTokenResponse{Error: "access_denied"})
	}
}
