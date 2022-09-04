package model

import "time"

type Version struct {
	Time   time.Time `json:",omitempty"`
	Commit string    `json:",omitempty"`
	Tag    string    `json:",omitempty"`
}
