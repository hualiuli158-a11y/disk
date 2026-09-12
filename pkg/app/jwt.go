package app

import (
	"disk/pkg/setting"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID uint64 `json:"user_id"`
	jwt.RegisteredClaims
}

type JWTService struct {
	secret []byte
	issuer string
	expire time.Duration
}

func NewJWTService(config setting.JwtConfig) *JWTService {
	return &JWTService{
		secret: []byte(config.Secret),
		issuer: config.Issuer,
		expire: config.Expire,
	}
}

func (s *JWTService) GenerateToken(userID uint64) (string, error) {
	now := time.Now()

	claims := &Claims{
		UserID: userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.expire)),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    s.issuer,
		},
	}

	return jwt.NewWithClaims(
		jwt.SigningMethodHS256,
		claims,
	).SignedString(s.secret)
}

func (s *JWTService) ParseToken(token string) (*Claims, error) {
	tokenClaims, err := jwt.ParseWithClaims(
		token,
		&Claims{},
		func(token *jwt.Token) (any, error) {
			return s.secret, nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(s.issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return nil, err
	}

	claims, ok := tokenClaims.Claims.(*Claims)
	if !ok || !tokenClaims.Valid {
		return nil, jwt.ErrTokenInvalidClaims
	}

	return claims, nil
}
