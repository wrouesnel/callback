package models

import "github.com/jinzhu/gorm"

// Node represents endpoints which are registered with a callback server.
type Node struct {
	gorm.Model
	// Unique name of the node for connection
	NodeId string `gorm:"type:varchar(255);unique_index"`
	// CallbackServer which is currently hosting this node
	CallbackServer string `gorm:"type:text"`
	// If not blank, salted password in passwd format that the node must
	// present to connect to the server.
	Password string `gorm:"type:text"`
}
