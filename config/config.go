//Package config is responsible for keeping a relevant config of
//the gonzbee downloader.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
)

//The C variable is the general interface to the config package.
//
//You get the various settings from this variable which is populated
//at init time
var C Config

type Config struct {
	IncompleteDir string
	CompleteDir   string
	Server        ServerConfig
}

//ServerConfig holds the settings that describe connecting to a server
type ServerConfig struct {
	Address  string
	Port     int
	Username string
	Password string
}

//GetAddressStr returns the colon separated string of a serverconfigs
//address and port
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

//GetIncompleteDir returns the absolute directory path of the
//Incomplete directory. This directory will keep inprogress downloads.
//If the directory is not absolutely specified in the config,
//the home environment variable will be used as the base
func (c *Config) GetIncompleteDir() string {
	if !path.IsAbs(c.IncompleteDir) {
		return path.Join(os.Getenv("HOME"), c.IncompleteDir)
	}
	return c.IncompleteDir
}

//GetCompleteDir returns the absolute directory path of the
//Complete directory. This directory will keep completed downloads.
//If the directory is not absolutely specified in the config,
//the home environment variable will be used as the base
func (c *Config) GetCompleteDir() string {
	if !path.IsAbs(c.CompleteDir) {
		return path.Join(os.Getenv("HOME"), c.CompleteDir)
	}
	return c.CompleteDir
}


//this is the default config that will be used if no config file could be
//found
var defaultConfig = Config{
	IncompleteDir: "gonzbee/incomplete",
	CompleteDir:   "gonzbee/complete",
}

func init() {
	//this is very unix specific, beware eventual porters
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		panic(errors.New("Cannot Get Config: No home Directory"))
	}
	configDir := path.Join(homeDir, ".gonzbee")
	err := os.Mkdir(configDir, 0777)
	if err != nil && !isEEXIST(err) {
		panic(fmt.Errorf("Cannot Get Config: %s", err.Error()))
	}
	//check if a config file exists
	configPath := path.Join(configDir, "config")
	err = readConfigFile(configPath)
	if err != nil {
		panic(fmt.Errorf("Cannot Get Config: %s", err.Error()))
	}
	err = C.setup()
	if err != nil {
		panic(fmt.Errorf("Cannot Get Config: %s", err.Error()))
	}
}

func readConfigFile(path string) error {
	file, created, err := openOrCreate(path)
	if err != nil {
		return err
	}
	defer file.Close()

	if created {
		return firstConfig(file)
	}
	return existingConfig(file)
}

func firstConfig(file *os.File) error {
	config, err := json.MarshalIndent(defaultConfig, "", "\t")
	if err != nil {
		return err
	}
	_, err = file.Write(config)
	if err != nil {
		return err
	}
	C = defaultConfig
	return err
}

func existingConfig(file *os.File) error {
	enc := json.NewDecoder(file)
	err := enc.Decode(&C)
	return err
}

func openOrCreate(path string) (*os.File, bool, error) {
	file, err := os.OpenFile(path, os.O_EXCL|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil && isEEXIST(err) {
		file, err = os.Open(path)
		return file, false, err
	}
	return file, true, err
}

func (c *Config) setup() error {
	return c.createDownloadDirs()
}

func (c *Config) createDownloadDirs() error {
	dirPath := c.IncompleteDir
	if !path.IsAbs(dirPath) {
		dirPath = path.Join(os.Getenv("HOME"), dirPath)
	}
	err := os.MkdirAll(dirPath, 0777)
	if err != nil {
		return err
	}

	dirPath = c.CompleteDir
	if !path.IsAbs(dirPath) {
		dirPath = path.Join(os.Getenv("HOME"), dirPath)
	}
	err = os.MkdirAll(dirPath, 0777)
	if err != nil {
		return err
	}
	return nil
}

func isEEXIST(err error) bool {
	perr := err.(*os.PathError)
	return perr.Err == os.EEXIST
}
