package main

import (
	"errors"
	"fmt"
	
"github.com/golang-jwt/jwt/v5"

)

func validateJWT(tokenStr string) (map[string]interface{}, error) {
	if tokenStr == "" {
		return nil, errors.New("empty token")
	}
	
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtSecret, nil
	})
	
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("invalid claims type")
	}
	
	result := make(map[string]interface{})
	for k, v := range claims {
		result[k] = v
	}
	return result, nil
}