// Package future provides a Future type that can be used to
// represent a value that will be available in the future.
package future

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// WeatherDataResult be used in GetWeatherData.
type WeatherDataResult struct {
	Value interface{}
	Err   error
}

type Future struct {
	result chan interface{}
}

func NewFuture() *Future {
	return &Future{
		result: make(chan interface{}, 1),
	}
}

// sets the future contents
func (f *Future) CompleteFuture(res interface{}) {
	f.result <- res
	f.CloseFuture()
}

// extacts and returns the contents of the future
// blocks until the contents are available
func (f *Future) GetResult() interface{} {
	return <-f.result
}

// closes the channel
func (f *Future) CloseFuture() {
	close(f.result)
}

// Wait waits for the first n futures to return or for the timeout to expire,
// whichever happens first.
func Wait(futures []*Future, n int, timeout time.Duration, postCompletionLogic func(interface{}) bool) []interface{} {
	res := []interface{}{}
	completed := 0
	ch := make(chan interface{}, len(futures))

	for _, fut := range futures {
		go func(f *Future) {
			result := f.GetResult()
			ch <- result
		}(fut)
	}

	timeoutCh := time.After(timeout)

	for completed < n {
		select {
		case result := <-ch:
			if postCompletionLogic == nil || postCompletionLogic(result) {
				res = append(res, result)
				completed++
			}
		case <-timeoutCh:
			return res
		}
	}

	return res
}

// User Defined Function Logic

// GetWeatherData implementation which immediately returns a Future.
func GetWeatherData(baseURL string, id int) *Future {
	future := NewFuture()

	go func() {
		url := fmt.Sprintf("%s/weather?id=%d", baseURL, id)

		rsp, err := http.Get(url)
		if err != nil {
			future.CompleteFuture(WeatherDataResult{Err: err})
			return
		}

		var weatherData interface{}
		err = json.NewDecoder(rsp.Body).Decode(&weatherData)
		if err != nil {
			future.CompleteFuture(WeatherDataResult{Err: err})
			return
		}
		future.CompleteFuture(WeatherDataResult{Value: weatherData})
		defer rsp.Body.Close()
	}()

	// Return the Future immediately
	return future
}

// heatWaveWarning is the PostCompletionLogic function for the received weatherData.
// Should be used to filter out all temperatures > 35 degrees Celsius.
func heatWaveWarning(res interface{}) bool {
	if data, ok := res.(WeatherDataResult); ok {
		if data.Err != nil {
			return false
		}
		if temp, ok := data.Value.(float64); ok {
			return temp > 35
		}
	}
	return false
}
