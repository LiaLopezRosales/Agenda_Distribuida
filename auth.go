// auth.go
package agendadistribuida

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// ======================
// Configuración JWT
// ======================

// Clave secreta para firmar los tokens (⚠️ en producción usar env var segura)
var jwtKey = []byte("supersecretkey")

// Claims personalizados para JWT
type Claims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// ======================
// Passwords
// ======================

// Genera hash seguro a partir de un password
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// Verifica un password contra su hash
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// ======================
// JWT helpers
// ======================

// Genera un JWT válido por 24 horas
func GenerateToken(user *User) (string, error) {
	expiration := time.Now().Add(24 * time.Hour)
	claims := &Claims{
		UserID:   user.ID,
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiration),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtKey)
}

// Parsea y valida un JWT, retornando Claims
func ParseToken(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return jwtKey, nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("invalid or expired token")
	}
	return claims, nil
}

// ======================
// Funciones de Autenticación
// ======================

// Autentica un usuario con username/password y genera JWT si es válido
func AuthenticateUser(storage *Storage, username, password string) (*User, string, error) {
	user, err := storage.GetUserByUsername(username)
	if err != nil {
		return nil, "", errors.New("invalid username or password")
	}
	if !CheckPasswordHash(password, user.PasswordHash) {
		return nil, "", errors.New("invalid username or password")
	}
	token, err := GenerateToken(user)
	if err != nil {
		return nil, "", err
	}
	return user, token, nil
}

// ======================
// Middleware de Autenticación
// ======================

type contextKey string

var userContextKey = contextKey("user")

// Middleware: valida Authorization: Bearer <token> y carga User en contexto
func AuthMiddleware(next http.Handler, storage *Storage) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Missing Authorization header", http.StatusUnauthorized)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Invalid Authorization format", http.StatusUnauthorized)
			return
		}
		tokenStr := parts[1]

		claims, err := ParseToken(tokenStr)
		if err != nil {
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Buscar usuario en la base de datos
		user, err := storage.GetUserByUsername(claims.Username)
		if err != nil {
			http.Error(w, "User not found", http.StatusUnauthorized)
			return
		}

		// Guardar en contexto
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ======================
// Utilidades para Handlers
// ======================

// Recupera el usuario autenticado desde el contexto
func GetUserFromContext(r *http.Request) (*User, error) {
	u := r.Context().Value(userContextKey)
	if u == nil {
		return nil, errors.New("no user in context")
	}
	user, ok := u.(*User)
	if !ok {
		return nil, errors.New("invalid user type in context")
	}
	return user, nil
}
