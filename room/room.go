package room

import (
	"fmt"
	"time"

	"github.com/j18e/hs110"
	"github.com/j18e/tempctl/models"
	"github.com/j18e/tempctl/storage"
	log "github.com/sirupsen/logrus"
)

type Room struct {
	Name       string
	Users      []*models.User
	TargetTemp float64
	PlugAddr   string
	StartTime  time.Duration // duration after midnight to become active
	StopTime   time.Duration // duration after midnight to stop being active
	Storage    storage.Storage

	plug *hs110.Plug
}

func (r *Room) Init() error {
	// check name
	if r.Name == "" {
		return fmt.Errorf("room requires a name")
	}

	// init plug
	r.plug = hs110.NewPlug(r.PlugAddr)

	// check start and stop times
	if r.StartTime > r.StopTime {
		return fmt.Errorf("StartTime cannot be greater than StopTime")
	} else if r.StopTime > time.Hour*24 {
		return fmt.Errorf("StopTime must be less than 24h")
	} else if r.StartTime < 0 {
		return fmt.Errorf("StartTime must be >= 0")
	}

	return nil
}

func (r *Room) Check() error {
	// only run loop during active hours
	if !r.activeHours() {
		r.Cool()
		return nil
	}

	// check if someone's home
	var someoneHome bool
	if len(r.Users) == 0 {
		someoneHome = true
	} else {
		var err error
		someoneHome, err = r.Storage.SomeonePresent(r.Users)
		if err != nil {
			return fmt.Errorf("checking for present user: %w", err)
		}
	}

	// turn off the plug if we can't get the current temp
	temp, err := r.Storage.CurrentTemp(r.Name)
	if err != nil {
		return fmt.Errorf("getting current temp: %w", err)
	}

	// start heating if someone's home and it's too cold
	if someoneHome && temp < r.TargetTemp {
		r.Heat()
		return nil
	}

	r.Cool()
	return nil
}

func (r *Room) Heat() {
	if err := r.plug.On(); err != nil {
		log.Warnf("heat room %s: %v", r.Name, err)
	} else {
		log.Infof("room %s: heating", r.Name)
	}
	if err := r.Storage.WriteHeatingStatus(r.Name, true); err != nil {
		log.Warnf("room %s: writing heating event: %v", r.Name, err)
	}
}

func (r *Room) Cool() {
	if err := r.plug.Off(); err != nil {
		log.Warnf("cool room %s: %v", r.Name, err)
	} else {
		log.Infof("room %s: cooling", r.Name)
	}
	if err := r.Storage.WriteHeatingStatus(r.Name, false); err != nil {
		log.Warnf("room %s: writing heating event: %v", r.Name, err)
	}
}

// ActiveHours reports whether the room is currently inside its configured
// active hours.
func (r *Room) activeHours() bool {
	if r.StartTime == 0 && r.StopTime == 0 {
		return true
	}

	t := time.Now()
	now := time.Hour*time.Duration(t.Hour()) + time.Minute*time.Duration(t.Minute())

	if now > r.StartTime && now < r.StopTime {
		log.Debugf("inside active hours for the next %v", r.StopTime-now)
		return true
	}
	log.Debugf("outside active hours for the next %v", r.StartTime+time.Hour*24-now)
	return false
}
