package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/j18e/tempctl/models"
	"github.com/j18e/tempctl/room"
	"github.com/j18e/tempctl/storage"

	"github.com/ghodss/yaml"
	"github.com/jessevdk/go-flags"
	log "github.com/sirupsen/logrus"
)

func main() {
	var opts struct {
		Config     string `long:"config.file" required:"true" description:"path to the config file"`
		InfluxAddr string `long:"influx.address" required:"true" description:"influxdb server to connect to"`
		InfluxDB   string `long:"influx.db" required:"true" description:"database on influxdb server to connect to"`
		LogLevel   string `long:"log.level" default:"info" choice:"info" choice:"debug" description:"log level to use"`
		SyncFreq   int    `long:"sync.frequency" default:"30" description:"time in seconds between checks"`
	}

	_, err := flags.Parse(&opts)
	if flags.WroteHelp(err) {
		os.Exit(0) // exit with zero status if help was called
	} else if _, ok := err.(*flags.Error); ok {
		os.Exit(1) // if it's a flags.Error the output is already printed
	} else if err != nil {
		log.Fatal(err)
	}

	switch opts.LogLevel {
	case "debug":
		log.SetLevel(log.DebugLevel)
	case "info":
		log.SetLevel(log.InfoLevel)
	default:
		log.Fatalf("unknown log level %s", opts.LogLevel)
	}

	rooms, err := config(opts.Config)
	if err != nil {
		log.Fatal(err)
	}

	// connect to storage
	stor, err := storage.NewInfluxStorage(opts.InfluxAddr, opts.InfluxDB)
	if err != nil {
		log.Fatalf("creating storage: %v", err)
	}
	defer stor.Close()

	// initialize rooms
	for _, r := range rooms {
		r.Storage = stor
		if err := r.Init(); err != nil {
			log.Fatal(err)
		}
	}
	log.Infof("initialized %d rooms", len(rooms))

	for {
		checkRooms(rooms)
		time.Sleep(time.Duration(opts.SyncFreq) * time.Second)
	}
}

func checkRooms(rooms []*room.Room) {
	errChan := make(chan error, len(rooms))
	defer close(errChan)
	for _, r := range rooms {
		go func(r *room.Room) {
			if err := r.Check(); err != nil {
				errChan <- fmt.Errorf("checking %s: %w", r.Name, err)
				return
			}
			errChan <- nil
		}(r)
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
		r := room.Room{
			Name:       rc.Name,
			Users:      users,
			TargetTemp: rc.TargetTemp,
			PlugAddr:   rc.PlugAddr,
			StartTime:  startDur,
			StopTime:   stopDur,
		}
		rooms = append(rooms, &r)
	}

	return rooms, nil
}
