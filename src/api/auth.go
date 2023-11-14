package api

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	jwt "github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
)

func ExtractToken(c *gin.Context) string {
	bearerToken := c.Request.Header.Get("Authorization")
	split := strings.Split(bearerToken, " ")
	if len(split) == 2 {
		return split[1]
	}
	return ""
}

func ExtractTokenID(c *gin.Context) error {
	tokenString := ExtractToken(c)
	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenString, &claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(os.Getenv("API_SECRET")), nil //TODO: replace this key
	})
	if err != nil {
		return fmt.Errorf("%s; token = %s", err.Error(), tokenString)
	}
	if !token.Valid {
		return errors.New("token is invalid")
	}
	sub, exists := claims["sub"]
	if !exists {
		return fmt.Errorf("no sub in token")
	}
	subString, ok := sub.(string)
	if !ok {
		return fmt.Errorf("sub in token is not a string")
	}
	c.Set("sub", subString)
	return nil
}

func JwtAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		err := ExtractTokenID(c)
		if err != nil {
			c.String(http.StatusUnauthorized, "Unauthorized: %s, secret: '%s'", err.Error(), os.Getenv("API_SECRET"))
			c.Abort()
			return
		}
		c.Next()
	}
}
