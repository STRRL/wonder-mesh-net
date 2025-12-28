package handlers

import (
	"encoding/json"
	"errors"
	"html"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/strrl/wonder-mesh-net/internal/app/coordinator/repository"
	"github.com/strrl/wonder-mesh-net/pkg/headscale"
)

var userCodePattern = regexp.MustCompile(`^[A-Z0-9]{4}-[A-Z0-9]{4}$`)
var deviceCodePattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

type DeviceHandler struct {
	publicURL               string
	deviceRequestRepository *repository.DeviceRequestRepository
	realmManager            *headscale.RealmManager
	authHelper              *AuthHelper
}

func NewDeviceHandler(
	publicURL string,
	deviceRequestRepository *repository.DeviceRequestRepository,
	realmManager *headscale.RealmManager,
	authHelper *AuthHelper,
) *DeviceHandler {
	return &DeviceHandler{
		publicURL:               publicURL,
		deviceRequestRepository: deviceRequestRepository,
		realmManager:            realmManager,
		authHelper:              authHelper,
	}
}

type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

func (h *DeviceHandler) HandleDeviceCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()
	req, err := h.deviceRequestRepository.Create(ctx)
	if err != nil {
		slog.Error("create device request", "error", err)
		http.Error(w, "create device request", http.StatusInternalServerError)
		return
	}

	resp := DeviceCodeResponse{
		DeviceCode:      req.DeviceCode,
		UserCode:        req.UserCode,
		VerificationURL: h.publicURL + "/coordinator/device/verify",
		ExpiresIn:       int(time.Until(req.ExpiresAt).Seconds()),
		Interval:        int(repository.PollInterval.Seconds()),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("encode device code response", "error", err)
	}
}

func (h *DeviceHandler) HandleDeviceVerifyPage(w http.ResponseWriter, r *http.Request) {
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

func (h *DeviceHandler) HandleDeviceVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	realm, err := h.authHelper.GetRealmFromRequest(ctx, r)
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

	if !userCodePattern.MatchString(req.UserCode) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid code format, expected XXXX-XXXX"})
		return
	}

	deviceReq, err := h.deviceRequestRepository.GetByUserCode(ctx, req.UserCode)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if errors.Is(err, repository.ErrDeviceRequestNotFound) {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid or expired code"})
		} else {
			slog.Error("get device request", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "internal error"})
		}
		return
	}

	if deviceReq.Status != repository.DeviceStatusPending {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "code already used"})
		return
	}

	key, err := h.realmManager.CreateAuthKeyByName(ctx, realm.HeadscaleUser, 24*time.Hour, false)
	if err != nil {
		slog.Error("create auth key for device", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "create auth key"})
		return
	}

	// Both headscaleURL and coordinatorURL use publicURL because the coordinator
	// reverse-proxies all Tailscale control plane traffic to the embedded Headscale
	// instance (see handlers/proxy.go). Workers use this single URL for both
	// coordinator API calls and tailscale --login-server.
	if err := h.deviceRequestRepository.Approve(ctx, req.UserCode, realm.ID, realm.HeadscaleUser, key.GetKey(), h.publicURL, h.publicURL); err != nil {
		slog.Error("approve device", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "approve device"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "approved"})
}

type DeviceTokenResponse struct {
	Authkey      string `json:"authkey,omitempty"`
	HeadscaleURL string `json:"headscale_url,omitempty"`
	User         string `json:"user,omitempty"`
	Error        string `json:"error,omitempty"`
}

func (h *DeviceHandler) HandleDeviceToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := r.Context()

	var req struct {
		DeviceCode string `json:"device_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(DeviceTokenResponse{Error: "invalid_request"})
		return
	}

	if !deviceCodePattern.MatchString(req.DeviceCode) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(DeviceTokenResponse{Error: "invalid_device_code_format"})
		return
	}

	deviceReq, err := h.deviceRequestRepository.GetByDeviceCode(ctx, req.DeviceCode)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		if errors.Is(err, repository.ErrDeviceRequestNotFound) {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(DeviceTokenResponse{Error: "invalid_device_code"})
		} else {
			slog.Error("get device request", "error", err)
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(DeviceTokenResponse{Error: "internal_error"})
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch deviceReq.Status {
	case repository.DeviceStatusPending:
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(DeviceTokenResponse{Error: "authorization_pending"})

	case repository.DeviceStatusApproved:
		h.deviceRequestRepository.Delete(ctx, req.DeviceCode)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(DeviceTokenResponse{
			Authkey:      deviceReq.Authkey,
			HeadscaleURL: deviceReq.HeadscaleURL,
			User:         deviceReq.HeadscaleUser,
		})

	case repository.DeviceStatusExpired:
		h.deviceRequestRepository.Delete(ctx, req.DeviceCode)
		w.WriteHeader(http.StatusGone)
		_ = json.NewEncoder(w).Encode(DeviceTokenResponse{Error: "expired_token"})

	case repository.DeviceStatusDenied:
		h.deviceRequestRepository.Delete(ctx, req.DeviceCode)
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(DeviceTokenResponse{Error: "access_denied"})
	}
}
