package schemas

import (
	"time"
)

type ServerStatus string

const (
	ServerInstalling ServerStatus = "INSTALLING"
	ServerRunning    ServerStatus = "RUNNING"
	ServerStopped    ServerStatus = "STOPPED"
	ServerSuspended  ServerStatus = "SUSPENDED"
	ServerError      ServerStatus = "ERROR"
)

type Servers struct {
	ID     string       `gorm:"primaryKey;type:varchar(55)" json:"id"`
	UUID   string       `gorm:"type:varchar(55);unique;not null" json:"uuid"`
	SID    string       `gorm:"unique;not null" json:"sid"`
	Name   string       `gorm:"type:varchar(64);unique;not null" json:"name"`
	Memory string       `gorm:"type:varchar(20);not null" json:"memory"`
	Cpu    int          `gorm:"not null" json:"cpu"`
	Disk   string       `gorm:"type:varchar(20);not null" json:"disk"`
	Status ServerStatus `gorm:"type:varchar(20);default:'installing'" json:"status"`

	UserID       string `gorm:"type:varchar(55)" json:"user_id"`
	AllocationID string `gorm:"type:varchar(55)" json:"allocation_id"`
	NodeID       string `gorm:"type:varchar(55)" json:"node_id"`

	// Many ServerBackups
	// Many ServerSchedules
	// Many ActivityLogs
	// One Server Allocation IP

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (Servers) TableName() string {
	return "servers"
}
