//Copyright 2012, Daniel Morsing
//For licensing information, See the LICENSE file

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
)

//The config variable is the general interface to the config package.
//
//You get the various settings from this variable which is populated
//at init time.
var config = newConfig()

//ServerConfig holds the settings that describe connecting to a server.
type ServerConfig struct {
	Address  string
	Port     int
	Username string
	Password string
	TLS      bool
}

//GetAddressStr returns the colon separated string of a serverconfigs
//address and port.
func (s *ServerConfig) GetAddressStr() string {
	if s.Address == "" {
		return ""
	}
	port := s.Port
	if port == 0 {
		port = 119
	}
	return fmt.Sprintf("%v:%d", s.Address, port)
}

// newConfig initialized config from a dotfile at $HOME/.gonzbee/config
func newConfig() *ServerConfig {
	//this is very unix specific, beware eventual porters
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		panic(errors.New("Cannot Get Config: No home Directory"))
	}
	configDir := path.Join(homeDir, ".gonzbee")
	err := os.Mkdir(configDir, 0777)
	if err != nil && !os.IsExist(err) {
		panic(fmt.Errorf("Cannot Get Config: %s", err.Error()))
	}
	//check if a config file exists
	configPath := path.Join(configDir, "config")
	c, err := readConfigFile(configPath)
	if err != nil {
		panic(fmt.Errorf("Cannot Get Config: %s", err.Error()))
	}
	return c
}

func readConfigFile(path string) (*ServerConfig, error) {
	file, created, err := openOrCreate(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	if created {
		return firstConfig(file)
	}
	return existingConfig(file)
}

func firstConfig(file *os.File) (*ServerConfig, error) {
	s := ServerConfig{}
	config, err := json.MarshalIndent(s, "", "\t")
	if err != nil {
		return nil, err
	}
	_, err = file.Write(config)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func existingConfig(file *os.File) (*ServerConfig, error) {
	c := new(ServerConfig)
	enc := json.NewDecoder(file)
	err := enc.Decode(c)
	if err != nil {
		return nil, err
	}
	return c, err
}

func openOrCreate(path string) (*os.File, bool, error) {
	file, err := os.OpenFile(path, os.O_EXCL|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil && os.IsExist(err) {
		file, err = os.Open(path)
		return file, false, err
	}
	return file, true, err
}
