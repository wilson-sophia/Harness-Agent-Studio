package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

const (
	idleDuration     = 30 * time.Minute
	absoluteDuration = 8 * time.Hour
	freshDuration    = 5 * time.Minute
	rememberDuration = 14 * 24 * time.Hour
)

type userRecord struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	PasswordHash string `json:"passwordHash"`
}

type sessionRecord struct {
	UserID     string    `json:"userId"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	VerifiedAt time.Time `json:"verifiedAt"`
	CSRF       string    `json:"csrf"`
}

type rememberRecord struct {
	UserID    string    `json:"userId"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type harnessCredential struct {
	AccountID      string `json:"accountId"`
	EncryptedToken string `json:"encryptedToken"`
}

type runRecord struct {
	ID          string    `json:"id"`
	UserID      string    `json:"userId"`
	Mode        string    `json:"mode"`
	AccountID   string    `json:"accountId"`
	OrgID       string    `json:"orgIdentifier"`
	ProjectID   string    `json:"projectIdentifier"`
	PipelineID  string    `json:"pipelineIdentifier"`
	ExecutionID string    `json:"executionId"`
	RepoURL     string    `json:"repoUrl"`
	CreatedAt   time.Time `json:"createdAt"`
}

type savedData struct {
	Users       map[string]userRecord        `json:"users"`
	Sessions    map[string]sessionRecord     `json:"sessions"`
	Remember    map[string]rememberRecord    `json:"remember"`
	Credentials map[string]harnessCredential `json:"credentials"`
	Runs        map[string]runRecord         `json:"runs"`
}

type application struct {
	mu            sync.Mutex
	data          savedData
	dataFile      string
	origin        string
	secureCookies bool
	encryptionKey []byte
	httpClient    *http.Client
	loginFailures map[string]loginAttempt
	inflight      map[string]bool
}

type loginAttempt struct {
	Count int
	Until time.Time
}

func newApplication() (*application, error) {
	origin := strings.TrimRight(getenv("APP_ORIGIN", "http://127.0.0.1:5173"), "/")
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Path != "" {
		return nil, errors.New("invalid APP_ORIGIN")
	}
	keyValue := os.Getenv("APP_ENCRYPTION_KEY")
	var key []byte
	if keyValue != "" {
		key, err = base64.StdEncoding.DecodeString(keyValue)
		if err != nil || len(key) != 32 {
			return nil, errors.New("APP_ENCRYPTION_KEY must be base64 for 32 bytes")
		}
	}
	path := getenv("DATA_FILE", "data/app.json")
	a := &application{
		dataFile: path, origin: origin, secureCookies: parsed.Scheme == "https", encryptionKey: key,
		httpClient:    &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		loginFailures: map[string]loginAttempt{},
		inflight:      map[string]bool{},
		data:          savedData{Users: map[string]userRecord{}, Sessions: map[string]sessionRecord{}, Remember: map[string]rememberRecord{}, Credentials: map[string]harnessCredential{}, Runs: map[string]runRecord{}},
	}
	raw, err := os.ReadFile(path)
	if err == nil {
		if err = json.Unmarshal(raw, &a.data); err != nil {
			return nil, fmt.Errorf("load data: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if a.data.Users == nil {
		a.data.Users = map[string]userRecord{}
	}
	if a.data.Sessions == nil {
		a.data.Sessions = map[string]sessionRecord{}
	}
	if a.data.Remember == nil {
		a.data.Remember = map[string]rememberRecord{}
	}
	if a.data.Credentials == nil {
		a.data.Credentials = map[string]harnessCredential{}
	}
	if a.data.Runs == nil {
		a.data.Runs = map[string]runRecord{}
	}
	return a, nil
}

func (a *application) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(a.dataFile), 0700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(a.data, "", "  ")
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(a.dataFile), ".app-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err = temp.Chmod(0600); err != nil {
		temp.Close()
		return err
	}
	if _, err = temp.Write(raw); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), a.dataFile)
}

func randomSecret() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func secretHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func (a *application) encryptToken(value string) (string, error) {
	if len(a.encryptionKey) != 32 {
		return "", errors.New("server encryption key is not configured")
	}
	block, err := aes.NewCipher(a.encryptionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(value), nil)), nil
}

func (a *application) decryptToken(value string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(a.encryptionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("invalid encrypted token")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	return string(plain), err
}

func (a *application) securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Cache-Control", "no-store")
		c.Header("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		c.Next()
	}
}

func (a *application) sameOrigin() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && origin == a.origin {
			c.Header("Access-Control-Allow-Origin", a.origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "GET,POST,DELETE,OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Content-Type,X-CSRF-Token")
		}
		if c.Request.Method == http.MethodOptions {
			if origin != a.origin {
				c.AbortWithStatus(http.StatusForbidden)
			} else {
				c.AbortWithStatus(http.StatusNoContent)
			}
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && origin != a.origin {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid request origin"})
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
		}
		c.Next()
	}
}

func (a *application) setCookie(c *gin.Context, name, value string, maxAge int) {
	http.SetCookie(c.Writer, &http.Cookie{Name: name, Value: value, Path: "/api", MaxAge: maxAge, Expires: time.Now().Add(time.Duration(maxAge) * time.Second), HttpOnly: true, Secure: a.secureCookies, SameSite: http.SameSiteStrictMode})
}

func (a *application) clearCookie(c *gin.Context, name string) {
	http.SetCookie(c.Writer, &http.Cookie{Name: name, Value: "", Path: "/api", MaxAge: -1, HttpOnly: true, Secure: a.secureCookies, SameSite: http.SameSiteStrictMode})
}

func (a *application) issueSession(c *gin.Context, userID string, verifiedAt time.Time, remember bool) (sessionRecord, error) {
	now := time.Now()
	id := randomSecret()
	session := sessionRecord{UserID: userID, CreatedAt: now, LastSeenAt: now, VerifiedAt: verifiedAt, CSRF: randomSecret()}
	a.data.Sessions[secretHash(id)] = session
	if remember {
		token := randomSecret()
		a.data.Remember[secretHash(token)] = rememberRecord{UserID: userID, ExpiresAt: now.Add(rememberDuration)}
		a.setCookie(c, "has_remember", token, int(rememberDuration.Seconds()))
	}
	if err := a.saveLocked(); err != nil {
		return session, err
	}
	a.setCookie(c, "has_session", id, int(idleDuration.Seconds()))
	return session, nil
}

func (a *application) currentSession(c *gin.Context, allowRemember bool) (sessionRecord, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	if cookie, err := c.Request.Cookie("has_session"); err == nil {
		key := secretHash(cookie.Value)
		if session, ok := a.data.Sessions[key]; ok {
			if now.Sub(session.CreatedAt) < absoluteDuration && now.Sub(session.LastSeenAt) < idleDuration {
				session.LastSeenAt = now
				a.data.Sessions[key] = session
				a.setCookie(c, "has_session", cookie.Value, int(idleDuration.Seconds()))
				_ = a.saveLocked()
				return session, true
			}
			delete(a.data.Sessions, key)
			_ = a.saveLocked()
		}
	}
	if !allowRemember {
		return sessionRecord{}, false
	}
	cookie, err := c.Request.Cookie("has_remember")
	if err != nil {
		return sessionRecord{}, false
	}
	key := secretHash(cookie.Value)
	item, ok := a.data.Remember[key]
	if !ok || now.After(item.ExpiresAt) {
		delete(a.data.Remember, key)
		_ = a.saveLocked()
		return sessionRecord{}, false
	}
	delete(a.data.Remember, key)
	session, err := a.issueSession(c, item.UserID, time.Time{}, false)
	if err != nil {
		return sessionRecord{}, false
	}
	replacement := randomSecret()
	a.data.Remember[secretHash(replacement)] = item
	if err = a.saveLocked(); err != nil {
		return sessionRecord{}, false
	}
	a.setCookie(c, "has_remember", replacement, int(time.Until(item.ExpiresAt).Seconds()))
	return session, true
}

func (a *application) requireSession(fresh bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := a.currentSession(c, true)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "sign in required"})
			return
		}
		if c.Request.Method != http.MethodGet && c.GetHeader("X-CSRF-Token") != session.CSRF {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid CSRF token"})
			return
		}
		if fresh && (session.VerifiedAt.IsZero() || time.Since(session.VerifiedAt) > freshDuration) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "recent password confirmation required", "code": "FRESH_AUTH_REQUIRED"})
			return
		}
		c.Set("session", session)
		c.Next()
	}
}

func sessionFrom(c *gin.Context) sessionRecord { return c.MustGet("session").(sessionRecord) }

func (a *application) auth(c *gin.Context, register bool) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
		Remember bool   `json:"remember"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid credentials"})
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if len(email) > 254 || !strings.Contains(email, "@") || len(req.Password) < 10 || len(req.Password) > 128 {
		c.JSON(400, gin.H{"error": "valid email and password of 10-128 characters required"})
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var user userRecord
	if register {
		for _, existing := range a.data.Users {
			if existing.Email == email {
				c.JSON(409, gin.H{"error": "account already exists"})
				return
			}
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(500, gin.H{"error": "registration failed"})
			return
		}
		user = userRecord{ID: randomSecret(), Email: email, PasswordHash: string(hash)}
		a.data.Users[user.ID] = user
	} else {
		attemptKey := email + "|" + c.ClientIP()
		attempt := a.loginFailures[attemptKey]
		if attempt.Count >= 5 && time.Now().Before(attempt.Until) {
			c.JSON(429, gin.H{"error": "too many sign-in attempts; try again later"})
			return
		}
		for _, existing := range a.data.Users {
			if existing.Email == email {
				user = existing
				break
			}
		}
		if user.ID == "" || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
			if time.Now().After(attempt.Until) {
				attempt = loginAttempt{Until: time.Now().Add(15 * time.Minute)}
			}
			attempt.Count++
			a.loginFailures[attemptKey] = attempt
			c.JSON(401, gin.H{"error": "invalid email or password"})
			return
		}
		delete(a.loginFailures, attemptKey)
	}
	session, err := a.issueSession(c, user.ID, time.Now(), req.Remember)
	if err != nil {
		c.JSON(500, gin.H{"error": "session could not be saved"})
		return
	}
	c.JSON(200, gin.H{"email": user.Email, "csrfToken": session.CSRF, "freshUntil": session.VerifiedAt.Add(freshDuration)})
}

func (a *application) me(c *gin.Context) {
	session, ok := a.currentSession(c, true)
	if !ok {
		c.JSON(200, gin.H{"authenticated": false})
		return
	}
	a.mu.Lock()
	user := a.data.Users[session.UserID]
	a.mu.Unlock()
	c.JSON(200, gin.H{"authenticated": true, "email": user.Email, "csrfToken": session.CSRF, "freshUntil": session.VerifiedAt.Add(freshDuration)})
}

func (a *application) reauthenticate(c *gin.Context) {
	var req struct {
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "password required"})
		return
	}
	session := sessionFrom(c)
	a.mu.Lock()
	user := a.data.Users[session.UserID]
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
		a.mu.Unlock()
		c.JSON(401, gin.H{"error": "invalid password"})
		return
	}
	cookie, _ := c.Request.Cookie("has_session")
	session.VerifiedAt = time.Now()
	a.data.Sessions[secretHash(cookie.Value)] = session
	err := a.saveLocked()
	a.mu.Unlock()
	if err != nil {
		c.JSON(500, gin.H{"error": "session could not be saved"})
		return
	}
	c.JSON(200, gin.H{"freshUntil": session.VerifiedAt.Add(freshDuration)})
}

func (a *application) logout(c *gin.Context) {
	session := sessionFrom(c)
	a.mu.Lock()
	if cookie, err := c.Request.Cookie("has_session"); err == nil {
		delete(a.data.Sessions, secretHash(cookie.Value))
	}
	if cookie, err := c.Request.Cookie("has_remember"); err == nil {
		delete(a.data.Remember, secretHash(cookie.Value))
	}
	_ = session
	_ = a.saveLocked()
	a.mu.Unlock()
	a.clearCookie(c, "has_session")
	a.clearCookie(c, "has_remember")
	c.Status(204)
}
