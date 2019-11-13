// Package events provides simple EventEmitter support for Go Programming Language
package events

import (
	"log"
	"reflect"
	"runtime"
	"sync"
)

const (
	// Version current version number
	Version = "0.0.4"
	// DefaultMaxListeners is the number of max listeners per event
	// default EventEmitters will print a warning if more than x listeners are
	// added to it. This is a useful default which helps finding memory leaks.
	// Defaults to 0, which means unlimited
	DefaultMaxListeners = 0

	// EnableWarning prints a warning when trying to add an event which it's len is equal to the maxListeners
	// Defaults to false, which means it does not print a warning
	EnableWarning = false
)

type (
	// Listener is the type of a Listener, it's a func which receives any,optional, arguments from the caller/Emitter
	Listener func(string, ...interface{})
	// Events the type for registered listeners, it's just a map[string][]func(...interface{})
	Events map[string][]Listener

	// EventEmitter is the message/or/event manager
	EventEmitter interface {
		// AddListener is an alias for .On(string, listener).
		AddListener(string, ...Listener)
		// Emit fires a particular event,
		// Synchronously calls each of the listeners registered for the event named
		// eventName, in the order they were registered,
		// passing the supplied arguments to each.
		Emit(string, ...interface{})
		// Exists checks that listener for particular event exists.
		Exists(string) bool
		// EventNames returns an array listing the events for which the emitter has registered listeners.
		// The values in the array will be strings.
		EventNames() []string
		// GetMaxListeners returns the max listeners for this Emitter
		// see SetMaxListeners
		GetMaxListeners() int
		// ListenerCount returns the length of all registered listeners to a particular event
		ListenerCount(string) int
		// Listeners returns a copy of the array of listeners for the event named eventName.
		Listeners(string) []Listener
		// On registers a particular listener for an event, func receiver parameter(s) is/are optional
		On(string, ...Listener)
		// Once adds a one time listener function for the event named eventName.
		// The next time eventName is triggered, this listener is removed and then invoked.
		Once(string, ...Listener)
		// Sole adds a sole listener for this event by removing all listeners for the event before adding new listener
		Sole(string, ...Listener)
		// RemoveAllListeners removes all listeners, or those of the specified eventName.
		// Note that it will remove the event itself.
		// Returns an indicator if event and listeners were found before the remove.
		RemoveAllListeners(string) bool
		// RemoveListener removes given listener from the event named eventName.
		// Returns an indicator whether listener was removed
		RemoveListener(string, Listener) bool
		// Clear removes all events and all listeners, restores Events to an empty value
		Clear()
		// SetMaxListeners obviously this function allows the MaxListeners
		// to be decrease or increase. Set to zero for unlimited
		SetMaxListeners(int)
		// Len returns the length of all registered events
		Len() int
		// Stats returns stats snapshot
		Stats() *stats
	}

	emitter struct {
		stats        stats
		maxListeners int
		evtListeners Events
		mu           sync.Mutex
	}
)

// CopyTo copies the event listeners to an EventEmitter
func (e Events) CopyTo(emitter EventEmitter) {
	if e != nil && len(e) > 0 {
		// register the events to/with their listeners
		for evt, listeners := range e {
			if len(listeners) > 0 {
				emitter.AddListener(evt, listeners...)
			}
		}
	}
}

// New returns a new, empty, EventEmitter
func New() EventEmitter {
	return &emitter{maxListeners: DefaultMaxListeners, evtListeners: Events{}}
}

var (
	_              EventEmitter = &emitter{}
	defaultEmitter              = New()
)

// AddListener is an alias for .On(eventName, listener).
func AddListener(evt string, listener ...Listener) {
	defaultEmitter.AddListener(evt, listener...)
}

func (e *emitter) AddListener(evt string, listener ...Listener) {
	if len(listener) == 0 {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.evtListeners == nil {
		e.evtListeners = Events{}
	}

	listeners := e.evtListeners[evt]

	if e.maxListeners > 0 && len(listeners) == e.maxListeners {
		if EnableWarning {
			log.Printf(`(events) warning: possible EventEmitter memory '
                    leak detected. %d listeners added. '
                    Use emitter.SetMaxListeners(n int) to increase limit.`, len(listeners))
		}
		return
	}

	if listeners == nil {
		listeners = make([]Listener, e.maxListeners)
	}

	e.stats.incSubscribers(len(listener))
	e.evtListeners[evt] = append(listeners, listener...)
}

// Emit fires a particular event,
// Synchronously calls each of the listeners registered for the event named
// eventName, in the order they were registered,
// passing the supplied arguments to each.
func Emit(evt string, data ...interface{}) {
	defaultEmitter.Emit(evt, data...)
}

func (e *emitter) Emit(evt string, data ...interface{}) {
	if e.evtListeners == nil {
		return // has no listeners to emit/speak yet
	}
	if listeners, ok := e.evtListeners[evt]; ok {
		for i := range listeners {
			l := listeners[i]
			if l != nil {
				go callListenerWithRecover(l, string(evt), data...)
				e.stats.incFiredEvents()
			}
		}
	}
}

// Exists checks that listener for particular event exists.
// This method uses default emitter
func Exists(evt string) bool {
	return defaultEmitter.Exists(evt)
}

// Exists checks that listener for particular event exists.
func (e *emitter) Exists(evt string) bool {
	if e.evtListeners == nil {
		return false
	}
	_, ok := e.evtListeners[evt]
	return ok
}

func callListenerWithRecover(listener Listener, event string, data ...interface{}) {
	defer func() {
		if x := recover(); x != nil {
			stackTrace := make([]byte, 1<<20)
			n := runtime.Stack(stackTrace, false)
			log.Printf("events.On: %s. Panic occured: %v\nStack trace: %s", event, x, stackTrace[:n])
		}
	}()
	listener(event, data...)
}

// EventNames returns an array listing the events for which the emitter has registered listeners.
// The values in the array will be strings.
func EventNames() []string {
	return defaultEmitter.EventNames()
}

func (e *emitter) EventNames() []string {
	if e.evtListeners == nil || e.Len() == 0 {
		return nil
	}

	names := make([]string, e.Len(), e.Len())
	i := 0
	for k := range e.evtListeners {
		names[i] = k
		i++
	}
	return names
}

// GetMaxListeners returns the max listeners for this Emitter
// see SetMaxListeners
func GetMaxListeners() int {
	return defaultEmitter.GetMaxListeners()
}

func (e *emitter) GetMaxListeners() int {
	return e.maxListeners
}

// ListenerCount returns the length of all registered listeners to a particular event
func ListenerCount(evt string) int {
	return defaultEmitter.ListenerCount(evt)
}

func (e *emitter) ListenerCount(evt string) int {
	if e.evtListeners == nil {
		return 0
	}
	len := 0

	if evtListeners, ok := e.evtListeners[evt]; ok {
		for _, l := range evtListeners {
			if l == nil {
				continue
			}
			len++
		}
	}

	return len
}

// Listeners returns a copy of the array of listeners for the event named eventName.
func Listeners(evt string) []Listener {
	return defaultEmitter.Listeners(evt)
}

func (e *emitter) Listeners(evt string) []Listener {
	if e.evtListeners == nil {
		return nil
	}
	var listeners []Listener
	if evtListeners, ok := e.evtListeners[evt]; ok {
		for _, l := range evtListeners {
			if l == nil {
				continue
			}

			listeners = append(listeners, l)
		}

		if len(listeners) > 0 {
			return listeners
		}
	}

	return nil
}

// Sole registers a sole listener for the event
func Sole(evt string, listener ...Listener) {
	defaultEmitter.Sole(evt, listener...)
}

func (e *emitter) Sole(evt string, listener ...Listener) {
	e.RemoveAllListeners(evt)
	e.AddListener(evt, listener...)
}

// On registers a particular listener for an event, func receiver parameter(s) is/are optional
func On(evt string, listener ...Listener) {
	defaultEmitter.On(evt, listener...)
}

func (e *emitter) On(evt string, listener ...Listener) {
	e.AddListener(evt, listener...)
}

// Once adds a one time listener function for the event named eventName.
// The next time eventName is triggered, this listener is removed and then invoked.
func Once(evt string, listener ...Listener) {
	defaultEmitter.Once(evt, listener...)
}

// RemoveListener removes given listener from the event named eventName.
func RemoveListener(evt string, listener Listener) {
	defaultEmitter.RemoveListener(evt, listener)
}

func (e *emitter) Once(evt string, listener ...Listener) {
	if len(listener) == 0 {
		return
	}

	var modifiedListeners []Listener

	if e.evtListeners == nil {
		e.evtListeners = Events{}
	}

	for i, l := range listener {

		idx := len(e.evtListeners) + i // get the next index (where this event should be added) and adds the i for the 'capacity'

		func(listener Listener, index int) {
			fired := false
			// remove the specific listener from the listeners before fire the real listener
			modifiedListeners = append(modifiedListeners, func(name string, data ...interface{}) {
				if e.evtListeners == nil {
					return
				}
				if !fired {
					// make sure that we don't get a panic(index out of array or nil map here
					if e.evtListeners[evt] != nil && (len(e.evtListeners[evt]) > index || index == 0) {

						e.mu.Lock()
						//e.evtListeners[evt] = append(e.evtListeners[evt][:index], e.evtListeners[evt][index+1:]...)
						// we do not touch the order because of the pre-defined indexes, we need just to make this listener nil in order to be not executed,
						// and make the len of listeners increase when listener is not nil, not just the len of listeners.
						// so set this listener to nil
						e.evtListeners[evt][index] = nil
						e.stats.decSubscribers()
						e.mu.Unlock()
					}
					fired = true
					listener(name, data...)
				}

			})
		}(l, idx)

	}
	e.AddListener(evt, modifiedListeners...)
}

// RemoveAllListeners removes all listeners, or those of the specified eventName.
// Note that it will remove the event itself.
// Returns an indicator if event and listeners were found before the remove.
func RemoveAllListeners(evt string) bool {
	return defaultEmitter.RemoveAllListeners(evt)
}

func (e *emitter) RemoveAllListeners(evt string) bool {
	if e.evtListeners == nil {
		return false // has nothing to remove
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if listeners := e.evtListeners[evt]; listeners != nil {
		l := e.ListenerCount(evt) // in order to not get the len of any inactive/removed listeners
		delete(e.evtListeners, evt)
		if l > 0 {
			return true
		}
	}

	return false
}

// RemoveListener removes the specified listener from the listener array for the event named eventName.
func (e *emitter) RemoveListener(evt string, listener Listener) bool {
	if e.evtListeners == nil {
		return false
	}

	if listener == nil {
		return false
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	listeners := e.evtListeners[evt]

	if listeners == nil {
		return false
	}

	idx := -1
	listenerPointer := reflect.ValueOf(listener).Pointer()

	for index, item := range listeners {
		itemPointer := reflect.ValueOf(item).Pointer()
		if itemPointer == listenerPointer {
			idx = index
			break
		}
	}

	if idx < 0 {
		return false
	}

	e.stats.decSubscribers()

	var modifiedListeners []Listener

	if len(listeners) > 1 {
		modifiedListeners = append(listeners[:idx], listeners[idx+1:]...)
	}

	e.evtListeners[evt] = modifiedListeners

	return true
}

// Clear removes all events and all listeners, restores Events to an empty value
func Clear() {
	defaultEmitter.Clear()
}

func (e *emitter) Clear() {
	e.evtListeners = Events{}
	e.stats.resetSubscribers()
}

// SetMaxListeners obviously this function allows the MaxListeners
// to be decrease or increase. Set to zero for unlimited
func SetMaxListeners(n int) {
	defaultEmitter.SetMaxListeners(n)
}

func (e *emitter) SetMaxListeners(n int) {
	if n < 0 {
		if EnableWarning {
			log.Printf("(events) warning: MaxListeners must be positive number, tried to set: %d", n)
			return
		}
	}
	e.maxListeners = n
}

// Len returns the length of all registered events
func Len() int {
	return defaultEmitter.Len()
}

func (e *emitter) Len() int {
	if e.evtListeners == nil {
		return 0
	}
	return len(e.evtListeners)
}

// Stats return emitter stats
func Stats() *stats {
	return defaultEmitter.Stats()
}

func (e *emitter) Stats() *stats {
	return e.stats.snapshot()
}
