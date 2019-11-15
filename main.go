package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/ghodss/yaml"
	"github.com/j18e/tempctl/models"
	"github.com/j18e/tempctl/room"
	"github.com/j18e/tempctl/storage"
	log "github.com/sirupsen/logrus"
)

func main() {
	configFile := flag.String("config.file", "", "the config file to use")
	influxAddr := flag.String("influx.address", "", "address for the influxdb server")
	influxDB := flag.String("influx.db", "", "influx database to use")
	logLevel := flag.String("log.level", "info", "log level to use")

	flag.Parse()

	if *configFile == "" {
		log.Fatal("required flag -config.file")
	} else if *influxAddr == "" {
		log.Fatal("required flag -influx.address")
	} else if *influxDB == "" {
		log.Fatal("required flag -influx.db")
	}

	switch *logLevel {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	default:
		log.Fatalf("unknown log.level %s", *logLevel)
	}

	rooms, err := config(*configFile)
	if err != nil {
		log.Fatal(err)
	}

	// connect to storage
	stor, err := storage.NewInfluxStorage(*influxAddr, *influxDB)
	if err != nil {
		log.Fatalf("creating storage: %v", err)
	}
	defer stor.Close()

	// initialize rooms
	for _, room := range rooms {
		room.Storage = stor
		if err := room.Init(); err != nil {
			log.Fatal(err)
		}
	}
	log.Infof("initialized %d rooms", len(rooms))

	for {
		checkRooms(rooms)
		time.Sleep(30 * time.Second)
	}
}

func checkRooms(rooms []*room.Room) {
	errChan := make(chan error, len(rooms))
	defer close(errChan)
	for _, room := range rooms {
		go func() {
			err := room.Check()
			if err != nil {
				errChan <- fmt.Errorf("checking %s: %w", room.Name, err)
				return
			}
			errChan <- nil
		}()
	}

	for range rooms {
		if err := <-errChan; err != nil {
			log.Errorf(err.Error())
		}
	}
}

func config(file string) ([]*room.Room, error) {
	const timeFormat = "15:04"

	var conf struct {
		Users map[string]string `json:"users"`
		Rooms []struct {
			Name       string   `json:"name"`
			PlugAddr   string   `json:"plug_address"`
			Occupants  []string `json:"occupants"`
			TargetTemp float64  `json:"target_temp"`
			StartTime  string   `json:"start_time"`
			StopTime   string   `json:"stop_time"`
		} `json:"rooms"`
	}

	var rooms []*room.Room

	bs, err := ioutil.ReadFile(file)
	if err != nil {
		return rooms, fmt.Errorf("reading file: %w", err)
	}

	if err := yaml.Unmarshal(bs, &conf); err != nil {
		return rooms, fmt.Errorf("unmarshaling file: %w", err)
	}

	for _, rc := range conf.Rooms {
		// parse start and stop times
		startTime, err := time.Parse(timeFormat, rc.StartTime)
		if err != nil {
			return rooms, fmt.Errorf("room %s: parsing start time: %w", rc.Name, err)
		}
		startDur := time.Hour*time.Duration(startTime.Hour()) + time.Minute*time.Duration(startTime.Minute())
		stopTime, err := time.Parse(timeFormat, rc.StopTime)
		if err != nil {
			return rooms, fmt.Errorf("room %s: parsing stop time: %w", rc.Name, err)
		}
		stopDur := time.Hour*time.Duration(stopTime.Hour()) + time.Minute*time.Duration(stopTime.Minute())

		// get users
		users := make([]*models.User, len(rc.Occupants))
		for i, name := range rc.Occupants {
			if _, ok := conf.Users[name]; !ok {
				return rooms, fmt.Errorf("room %s: no such user %s", rc.Name, name)
			}
			users[i] = &models.User{Name: name, MAC: conf.Users[name]}
		}

		// assemble room
		room := room.Room{
			Name:       rc.Name,
			Users:      users,
			TargetTemp: rc.TargetTemp,
			PlugAddr:   rc.PlugAddr,
			StartTime:  startDur,
			StopTime:   stopDur,
		}
		rooms = append(rooms, &room)
	}

	return rooms, nil
}
