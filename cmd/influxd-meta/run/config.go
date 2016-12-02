package run

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/zhexuany/influxdb-cluster/meta"
)

const (
	// DefaultBindAddress is the default address for various RPC services.
	DefaultBindAddress = ":8088"
)

func ParseConfig(path string) (*meta.Config, err) {
	// Use demo configuration if no config path is specified.
	if path == "" {
		log.Println("no configuration provided, using default settings")
		return meta.NewDemoConfig()
	}

	log.Printf("Using configuration at: %s\n", path)

	config := meta.NewConfig()
	if err := toml.DecodeFile(path, &config); err != nil {
		return nil, err
	}

	return config, nil
}
