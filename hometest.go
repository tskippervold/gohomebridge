package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/brutella/hc/service"

	"encoding/json"

	"github.com/brutella/hc"
	"github.com/brutella/hc/accessory"
)

type ConfigObject struct {
	Bridge      BridgeType
	Accessories []AccessoriesType
}

type BridgeType struct {
	Name     string
	Username string
	Port     int
	Pin      string
}

type AccessoriesType struct {
	Accessory   string
	Name        string
	Device_type string
	Proxy_id    string
	Base_url    string
}

type C4Light struct {
	Accessory *accessory.Lightbulb
	URL       string
	ProxyID   string
}

type C4Fan struct {
	*accessory.Accessory
	Fan     *service.Fan
	URL     string
	ProxyID string
}

func NewFan(info accessory.Info) *C4Fan {
	acc := C4Fan{}
	acc.Accessory = accessory.New(info, accessory.TypeFan)
	acc.Fan = service.NewFan()

	acc.Fan.On.SetValue(false)
	acc.AddService(acc.Fan.Service)

	return &acc
}

func loadConfigFromPath(path string) (ConfigObject, error) {
	file, err := ioutil.ReadFile(path)
	if err != nil {
		return ConfigObject{}, err
	}

	var confObj ConfigObject
	json.Unmarshal(file, &confObj)

	return confObj, nil
}

func UpdateLightCurrentState(light *C4Light) {
	actionURL := light.URL + "?command=get&proxyID=" + light.ProxyID + "&variableID=1001"
	response, err := http.Get(actionURL)
	if err != nil {
		fmt.Println(err)
		return
	}

	defer response.Body.Close()
	if response.StatusCode != 200 {
		return
	}

	bodyBytes, _ := ioutil.ReadAll(response.Body)

	data := make(map[string]interface{})
	err = json.Unmarshal(bodyBytes, &data)
	if err != nil {
		fmt.Println(err)
	}

	val, _ := data["1001"].(string)
	value, _ := strconv.Atoi(val)

	light.Accessory.Lightbulb.On.SetValue(value > 0)
	light.Accessory.Lightbulb.Brightness.SetValue(value)
}

func UpdateFanCurrentState(fan *C4Fan) {
	actionURL := fan.URL + "?command=get&proxyID=" + fan.ProxyID + "&variableID=1000"
	response, err := http.Get(actionURL)
	if err != nil {
		fmt.Println(err)
		return
	}

	defer response.Body.Close()
	if response.StatusCode != 200 {
		return
	}

	bodyBytes, _ := ioutil.ReadAll(response.Body)

	data := make(map[string]interface{})
	err = json.Unmarshal(bodyBytes, &data)
	if err != nil {
		fmt.Println(err)
	}

	val, _ := data["1000"].(int)
	var isOn bool
	if val == 1 {
		isOn = true
	} else {
		isOn = false
	}

	fan.Fan.On.SetValue(isOn)
}

func CreateC4Fan(accType *AccessoriesType) (C4Fan, error) {
	info := accessory.Info{
		Name:         accType.Name,
		Manufacturer: "Control4",
	}

	c4Fan := NewFan(info)
	c4Fan.URL = accType.Base_url
	c4Fan.ProxyID = accType.Proxy_id

	UpdateFanCurrentState(c4Fan)

	c4Fan.Fan.On.OnValueRemoteUpdate(func(power bool) {
		currentPower := c4Fan.Fan.On.GetValue()

		var actionURL string
		if power {
			actionURL = c4Fan.URL + "?command=set&proxyID=" + c4Fan.ProxyID + "&variableID=1000&newValue=1"
		} else {
			actionURL = c4Fan.URL + "?command=set&proxyID=" + c4Fan.ProxyID + "&variableID=1000&newValue=0"
		}

		response, err := http.Get(actionURL)

		if err != nil {
			fmt.Println(err)
			c4Fan.Fan.On.SetValue(currentPower)
			return
		}

		fmt.Println(response)
		c4Fan.Fan.On.SetValue(power)
	})

	return *c4Fan, nil
}

func CreateC4Light(accType *AccessoriesType) (C4Light, error) {
	info := accessory.Info{
		Name:         accType.Name,
		Manufacturer: "Control4",
	}

	acc := accessory.NewLightbulb(info)

	c4Light := &C4Light{
		Accessory: acc,
		URL:       accType.Base_url,
		ProxyID:   accType.Proxy_id}

	UpdateLightCurrentState(c4Light)

	acc.Lightbulb.On.OnValueRemoteUpdate(func(power bool) {
		brightness := func() int {
			if power {
				return 100
			}
			return 0
		}()

		currentBrightness := acc.Lightbulb.Brightness.GetValue()

		var actionURL string
		if power {
			actionURL = c4Light.URL + "?command=set&proxyID=" + c4Light.ProxyID + "&variableID=1001&newValue=100"
		} else {
			actionURL = c4Light.URL + "?command=set&proxyID=" + c4Light.ProxyID + "&variableID=1001&newValue=0"
		}

		response, err := http.Get(actionURL)

		if err != nil {
			fmt.Println(err)
			acc.Lightbulb.Brightness.SetValue(currentBrightness)
			return
		}

		fmt.Println(response)
		acc.Lightbulb.Brightness.SetValue(brightness)
	})

	acc.Lightbulb.Brightness.OnValueRemoteUpdate(func(value int) {
		actionURL := c4Light.URL + "?command=set&proxyID=" + c4Light.ProxyID + "&variableID=1001&newValue=" + strconv.Itoa(value)
		response, err := http.Get(actionURL)
		if err != nil {
			fmt.Println(err)
			return
		}

		fmt.Println(response)
	})

	return *c4Light, nil
}

func InitNewDevice(accType AccessoriesType) (interface{}, error) {

	if accType.Device_type == "light" {
		device, err := CreateC4Light(&accType)
		if err != nil {
			fmt.Println(err)
			return C4Light{}, err
		}

		return device, nil
	}
	if accType.Device_type == "fan" {
		device, err := CreateC4Fan(&accType)
		if err != nil {
			fmt.Println(err)
			return C4Fan{}, err
		}

		return device, nil
	}

	return C4Light{}, errors.New("Invalid")
}

func Connect() error {
	config, err := loadConfigFromPath("./accessories_config.json")
	if err != nil {
		return err
	}

	container := accessory.NewContainer()

	for _, accType := range config.Accessories {
		device, err := InitNewDevice(accType)
		if err != nil {
			fmt.Println(err)
			continue
		}

		if light, ok := device.(C4Light); ok {
			fmt.Println(light.ProxyID)
			container.AddAccessory(light.Accessory.Accessory)
		}
		if fan, ok := device.(C4Fan); ok {
			fmt.Println(fan.ProxyID)
			container.AddAccessory(fan.Accessory)
		}

	}

	bridgeInfo := accessory.Info{
		Name: config.Bridge.Name,
	}
	hcConfig := hc.Config{
		Pin: config.Bridge.Pin,
	}

	bridge := accessory.New(bridgeInfo, accessory.TypeBridge)

	t, err := hc.NewIPTransport(hcConfig, bridge, container.Accessories...)
	if err != nil {
		log.Fatal(err)
	}

	hc.OnTermination(t.Stop)
	t.Start()

	return nil
}

func main() {
	hc.OnTermination(func() {
		time.Sleep(100 * time.Millisecond)
		os.Exit(1)
	})

	if err := Connect(); err != nil {
		os.Exit(1)
	}
}
