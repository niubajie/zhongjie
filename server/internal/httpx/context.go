package httpx

import "github.com/gin-gonic/gin"

const (
	UserIDKey   = "userId"
	UserUUIDKey = "userUUID"
	RoleKey     = "role"
)

func SetUser(c *gin.Context, userID int64, userUUID, role string) {
	c.Set(UserIDKey, userID)
	c.Set(UserUUIDKey, userUUID)
	c.Set(RoleKey, role)
}

func CurrentUserID(c *gin.Context) int64 {
	v, ok := c.Get(UserIDKey)
	if !ok {
		return 0
	}
	id, _ := v.(int64)
	return id
}

func CurrentRole(c *gin.Context) string {
	v, ok := c.Get(RoleKey)
	if !ok {
		return ""
	}
	role, _ := v.(string)
	return role
}
