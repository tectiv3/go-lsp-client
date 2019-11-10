### Simple EventEmitter for Go Programming Language.

Overview
------------
`New() EventEmitter  // New returns a new, empty, EventEmitter`


```go
// AddListener is an alias for .On(eventName, listener).
AddListener(string, ...Listener)
// Emit fires a particular event,
// Synchronously calls each of the listeners registered for the event named
// eventName, in the order they were registered,
// passing the supplied arguments to each.
Emit(string, ...interface{})
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
// RemoveAllListeners removes all listeners, or those of the specified eventName.
// Note that it will remove the event itself.
// Returns an indicator if event and listeners were found before the remove.
RemoveAllListeners(string) bool
// Clear removes all events and all listeners, restores Events to an empty value
Clear()
// SetMaxListeners obviously this function allows the MaxListeners
// to be decrease or increase. Set to zero for unlimited
SetMaxListeners(int)
// Len returns the length of all registered events
Len() int
```


```go
import "github.com/xshellinc/isaax-go-events"

// initialize a new EventEmitter to use
e := events.New()

// register an event with name "my_event" and one listener
e.On("my_event", func(event string, payload ...interface{}) {
  message := payload[0].(string)
  print(message) // prints "this is my payload"
})

// fire the 'my_event' event
e.Emit("my_event", "this is my payload")

```

Default/global EventEmitter
```go

// register an event with name "my_event" and one listener to the global(package level) default EventEmitter
events.On("my_event", func(events string, payload ...interface{}) {
  message := payload[0].(string)
  print(message) // prints "this is my payload"
})

// fire the 'my_event' event
events.Emit("my_event", "this is my payload")

```

Remove an event

```go

events.On("my_event", func(event string, payload ...interface{}) {
  // first listener...
},func (event string, payload ...interface{}){
  // second listener...
})

println(events.Len()) // prints 1
println(events.ListenerCount("my_event")) // prints 2

// Remove our event, when/if we don't need this or we want to clear all of its listeners
events.RemoveAllListeners("my_event")

println(events.Len()) // prints 0
println(events.ListenerCount("my_event")) // prints 0
```


People
------------

[@kataras](https://github.com/kataras)
[@tectiv3](https://github.com/tectiv3)


License
------------

License can be found [here](LICENSE).
