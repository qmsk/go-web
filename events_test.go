package web

import (
	"math/rand"
	"sync"
	"testing"
	"time"
)

// run COUNT goroutines to read messages, every 0..INTERVAL
const READER_COUNT = 100
const READER_INTERVAL = 0.01 * float32(time.Second)

// ...which will each run for 0..TIME
const READER_TIME = 1.0 * float32(time.Second)

// ...and may delay processing of each event by 0..DELAY
const READER_DELAY = 0.01 * float32(time.Second)

// run COUNT goroutines to write messages...
const WRITER_COUNT = 5

// ...which will each write COUNT events at an interval of 0..1ms
const EVENT_INTERVAL = 0.001 * float32(time.Second)
const EVENT_COUNT = 1000

type testEvent struct {
	writer int
}

type testEvents struct {
	writeGroup sync.WaitGroup
	waitGroup  sync.WaitGroup
	eventChan  chan Event
	events     Events
}

// write events while sleeping
func (test *testEvents) writer(t *testing.T, i int) {
	defer test.writeGroup.Done()

	for count := 0; count <= EVENT_COUNT; count++ {
		time.Sleep(time.Duration(rand.Float32() * float32(EVENT_INTERVAL)))

		event := testEvent{writer: i}

		t.Logf("writer %d: write %v", i, event)
		test.eventChan <- event
	}
}

// read events while sleeping
func (test *testEvents) reader(t *testing.T, eventsClient eventsClient) {
	defer test.waitGroup.Done()

	var count = 0
	var startTime = time.Now()
	var stopChan = time.After(time.Duration(rand.Float32() * READER_TIME))

	for {
		select {
		case event, ok := <-eventsClient:
			if !ok {
				t.Logf("reader %p: closed @ %d messages after %v", eventsClient, count, time.Now().Sub(startTime))
				return
			} else {
				t.Logf("reader %p: read %v", eventsClient, event)

				count++

				// sleep 0..10ms while processing to trigger eventChan overflows
				time.Sleep(time.Duration(rand.ExpFloat64() * float64(READER_DELAY)))
			}

		case stopTime := <-stopChan:
			t.Logf("reader %p: stop @ %d messages after %v", eventsClient, count, stopTime.Sub(startTime))
			test.events.stop(eventsClient)
			return
		}
	}
}

func TestEvents(t *testing.T) {
	var test = testEvents{
		eventChan: make(chan Event),
	}

	test.events = MakeEvents(test.eventChan)

	// add followers
	test.waitGroup.Add(1)
	go func() {
		defer test.waitGroup.Done()

		for count := 0; count <= READER_COUNT; count++ {
			time.Sleep(time.Duration(rand.Float32() * READER_INTERVAL))

			eventsClient := test.events.listen()

			test.waitGroup.Add(1)
			go test.reader(t, eventsClient)
		}
	}()

	// add writers
	test.waitGroup.Add(1)
	go func() {
		defer test.waitGroup.Done()

		t.Log("Starting writers...")
		for i := 1; i < WRITER_COUNT; i++ {
			test.writeGroup.Add(1)
			go test.writer(t, i)
		}

		t.Log("Waiting writers...")
		test.writeGroup.Wait()

		t.Log("Completed writers...")
		//close(test.eventChan)
	}()

	t.Log("Running...")
	test.waitGroup.Wait()
}
