package gocelery

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// CeleryClient provides API for sending celery tasks
type CeleryClient struct {
	broker  CeleryBroker
	backend CeleryBackend
	worker  *CeleryWorker
}

// NewCeleryClient creates new celery client
func NewCeleryClient(broker CeleryBroker, backend CeleryBackend, numWorkers int) (*CeleryClient, error) {
	return &CeleryClient{
		broker,
		backend,
		NewCeleryWorker(broker, backend, numWorkers),
	}, nil
}

// Register task
func (cc *CeleryClient) Register(name string, task interface{}) {
	cc.worker.Register(name, task)
}

// StartWorker starts celery workers
func (cc *CeleryClient) StartWorker() {
	cc.worker.StartWorker()
}

// StopWorker stops celery workers
func (cc *CeleryClient) StopWorker() {
	cc.worker.StopWorker()
}

// Delay gets asynchronous result
func (cc *CeleryClient) Delay(task string, args ...interface{}) (*AsyncResult, error) {
	celeryTask := NewCeleryTask(task, args...)
	encodedMessage, err := celeryTask.Encode()
	if err != nil {
		return nil, err
	}
	celeryMessage := NewCeleryMessage(encodedMessage)
	err = cc.broker.Send(celeryMessage)
	if err != nil {
		return nil, err
	}
	return &AsyncResult{celeryTask.ID, cc.backend}, nil
}

// AsyncResult is pending result
type AsyncResult struct {
	taskID  string
	backend CeleryBackend
}

// Get gets actual result from redis
// It blocks for period of time set by timeout and return error if unavailable
func (ar *AsyncResult) Get(timeout time.Duration) (interface{}, error) {
	timeoutChan := time.After(timeout)
	for {
		select {
		case <-timeoutChan:
			err := fmt.Errorf("%v timeout getting result for %s", timeout, ar.taskID)
			return nil, err
		default:
			// process
			val, err := ar.AsyncGet()
			if err != nil {
				log.Printf("error getting result %v", err)
				continue
			}
			if val != nil {
				continue
			}
			return val, nil
		}
	}
}

// AsyncGet gets actual result from redis and returns nil if not available
func (ar *AsyncResult) AsyncGet() (interface{}, error) {
	// process
	val, err := ar.backend.GetResult(ar.taskID)
	if err != nil {
		return nil, err
	}
	if val == nil {
		return nil, err
	}
	var resMap map[string]interface{}
	json.Unmarshal(val.([]byte), &resMap)
	if resMap["status"] != "SUCCESS" {
		return nil, fmt.Errorf("error response status %v", resMap)
	}
	return resMap["result"], nil
}

// Ready checks if actual result is ready
func (ar *AsyncResult) Ready() (bool, error) {
	val, err := ar.backend.GetResult(ar.taskID)
	if err != nil {
		return false, err
	}
	return (val != nil), nil
}
