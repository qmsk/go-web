package web

import (
	"fmt"
	"net/http"

	"golang.org/x/net/websocket"
)

const EVENTS_BUFFER = 100

type State interface{}
type Event interface{}

type clientSet map[chan Event]bool

// add to set of clients
func (clientSet clientSet) register(clientChan chan Event) {
	clientSet[clientChan] = true
}

// remove from set on behalf of client requesting stop(); the clientChan may already be closed
func (clientSet clientSet) unregister(clientChan chan Event) {
	delete(clientSet, clientChan)
}

// remove from set on behalf of server; closes the clientChan to tell the client
//
// the client may trigger .unregister() later, which will be a no-op
func (clientSet clientSet) drop(clientChan chan Event) {
	close(clientChan)
	delete(clientSet, clientChan)
}

// write event to client, drop client if stuck
func (clientSet clientSet) send(clientChan chan Event, event Event) {
	select {
	case clientChan <- event:

	default:
		// client dropped behind
		clientSet.drop(clientChan)
	}
}

// distribute events to clients, dropping clients if they are stuck
func (clientSet clientSet) publish(event Event) {
	for clientChan, _ := range clientSet {
		clientSet.send(clientChan, event)
	}
}

func (clientSet clientSet) close() {
	for clientChan, _ := range clientSet {
		clientSet.drop(clientChan)
	}
}

type EventConfig struct {
	// recv from Events
	StateFunc func() State

	// send to Events
	EventPush <-chan Event
}

// WebSocket publish/subscribe
type Events struct {
	config         EventConfig
	registerChan   chan chan Event
	unregisterChan chan chan Event
}

// Publish events from chan
//
// Close chan to stop
func MakeEvents(config EventConfig) Events {
	events := Events{
		config:         config,
		registerChan:   make(chan chan Event),
		unregisterChan: make(chan chan Event),
	}

	go events.run(config)

	return events
}

func (events Events) run(config EventConfig) {
	clients := make(clientSet)
	defer clients.close()

	// panics any subscribed clients
	defer close(events.registerChan)
	defer close(events.unregisterChan)

	for {
		select {
		case clientChan := <-events.registerChan:
			clients.register(clientChan)

		case clientChan := <-events.unregisterChan:
			clients.unregister(clientChan)

		case event, ok := <-config.EventPush:
			if !ok {
				return
			}

			// log.Printf("web:Events: publish: %v", event)

			clients.publish(event)
		}
	}
}

// pull current state from sender
func (events Events) state() State {
	if events.config.StateFunc != nil {
		return events.config.StateFunc()
	} else {
		return struct{}{}
	}
}

// each subscriber has its own chan to receive from Events
type eventsClient chan Event

// Register new client
//
// recv on the returned chan
func (events Events) listen() (State, eventsClient) {
	eventChan := make(chan Event, EVENTS_BUFFER)

	events.registerChan <- eventChan

	return events.state(), eventChan
}

// Request server to stop sending us events
//
// XXX: panics with send on closed chan if server has stopped
func (events Events) stop(eventsClient eventsClient) {
	events.unregisterChan <- eventsClient
}

// Return error if aborting, nil if events closed
func (eventsClient eventsClient) serveWebsocket(websocketConn *websocket.Conn, state State) error {
	// initial state
	if err := websocket.JSON.Send(websocketConn, state); err != nil {
		return fmt.Errorf("webSocket.JSON.Send: %v", err)
	}

	// update events
	for event := range eventsClient {
		if err := websocket.JSON.Send(websocketConn, event); err != nil {
			return fmt.Errorf("webSocket.JSON.Send: %v", err)
		}
	}

	return nil
}

// goroutine-safe websocket subscriber
func (events Events) ServeWebsocket(websocketConn *websocket.Conn) {
	var state, eventsClient = events.listen()

	if err := eventsClient.serveWebsocket(websocketConn, state); err != nil {
		// stop, assuming that server is still alive
		// will panic if server has stopped
		events.stop(eventsClient)
	} else {
		// we do not need to request stop, server has unregistered us
	}
}

func (events Events) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	websocket.Handler(events.ServeWebsocket).ServeHTTP(w, r)
}
