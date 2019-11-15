package storage

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/influxdata/influxdb1-client" // this is important because of the bug in go mod
	influx "github.com/influxdata/influxdb1-client/v2"
	"github.com/j18e/tempctl/models"
)

func NewInfluxStorage(addr, db string) (*influxStorage, error) {
	var stor *influxStorage
	conn, err := influx.NewHTTPClient(influx.HTTPConfig{Addr: addr})
	if err != nil {
		return stor, fmt.Errorf("connecting to influxdb: %v", err)
	}
	if _, _, err := conn.Ping(time.Second * 5); err != nil {
		return stor, fmt.Errorf("pinging influxdb: %v", err)
	}
	return &influxStorage{
		conn:   conn,
		dbName: db,
	}, nil
}

type influxStorage struct {
	conn   influx.Client
	dbName string
}

func (s *influxStorage) Close() error {
	return s.conn.Close()
}

func (s *influxStorage) SomeonePresent(users []*models.User) (bool, error) {
	const qs = `SELECT last(uptime) FROM unifi_client WHERE time >= now() - 5m AND mac =~ /%s/`

	macs := make([]string, len(users))
	for i, u := range users {
		macs[i] = u.MAC
	}

	// query influxdb
	res, err := s.conn.Query(influx.NewQuery(fmt.Sprintf(qs, strings.Join(macs, "|")), s.dbName, "s"))
	if err != nil {
		return false, fmt.Errorf("querying influxdb: %w", err)
	}

	// check if any results were returned
	if len(res.Results[0].Series) > 0 {
		return true, nil
	}
	return false, nil
}

func (s *influxStorage) CurrentTemp(location string) (float64, error) {
	const qs = `select last(temperature) from environment where "location" = '%s' AND time >= now() - 10m`
	var temp float64

	// query influxdb
	res, err := s.conn.Query(influx.NewQuery(fmt.Sprintf(qs, location), s.dbName, "s"))
	if err != nil {
		return temp, fmt.Errorf("querying influxdb: %w", err)
	}

	// check if any results were returned
	if len(res.Results[0].Series) == 0 {
		return temp, fmt.Errorf("no temperature in last 10m with location %s", location)
	}

	// get temperature from result
	temp, err = res.Results[0].Series[0].Values[0][1].(json.Number).Float64()
	if err != nil {
		return temp, fmt.Errorf("converting temperature from json.Number: %w", err)
	}
	return temp, err
}

func (s *influxStorage) WriteHeatingStatus(location string, status int) error {
	const (
		measurement = "room_heating_status"
	)

	// validate the status
	switch status {
	case -1, 0, 1:
	default:
		return fmt.Errorf("valid status codes are [-1 0 1]")
	}

	// define fields, tags
	tags := map[string]string{"location": location}
	fields := map[string]interface{}{"status": status}

	// create batch point
	bp, err := influx.NewBatchPoints(influx.BatchPointsConfig{Database: s.dbName, Precision: "s"})
	if err != nil {
		return fmt.Errorf("creating influxdb batch points: %w", err)
	}

	pt, err := influx.NewPoint(measurement, tags, fields, time.Now())
	if err != nil {
		return fmt.Errorf("creating influxdb point: %w", err)
	}
	bp.AddPoint(pt)

	// write the point
	if err := s.conn.Write(bp); err != nil {
		return fmt.Errorf("writing points to influxdb: %w", err)
	}
	return nil
}
