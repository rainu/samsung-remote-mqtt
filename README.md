# samsung-remote-mqtt
A mqtt bridge for samsung smart tv. 

# Get the Binary
You can build it on your own (you will need [golang](https://golang.org/) installed):
```bash
go build -a -installsuffix cgo ./cmd/samsung-remote-mqtt/
```

Or you can download the release binaries: [here](https://github.com/rainu/samsung-remote-mqtt/releases/latest)

# usage
```bash
./samsung-remote-mqtt -broker tcp://127.0.0.1:1883 -tv-host TV_HOST_NAME
```

## Trigger command execution

To execute a command:

```bash
mosquitto_pub -t "cmnd/samsung-remote/info" -m ""
```

Read the result:
```bash
mosquitto_sub -t "cmnd/samsung-remote/info/result"
```

# Topics and Payload

|Topic|Payload|Description|
|-----|-------|-----------|
|%prefix%/info|-|Get information about the tv.|
|%prefix%/send-key|KEY_VOLUP|Send the key to samsung remote|
|%prefix%/send-text|SomeText|Send the given text to samsung remote|
|%prefix%/move|X Y|Go with the cursor to position samsung remote|
|%prefix%/right-click|-|Perform a right click at samsung remote|
|%prefix%/left-click|-|Perform a left click samsung remote|
|%prefix%/browser|https://github.com/rainu/samsung-remote-mqtt|Open the browser with the given url.|
|%prefix%/app/start|appId|Starts the app by the given **appId**.|
|%prefix%/app/start/%name%|appData|Starts the app by the given **name** with the given data. For example, you can start the YouTube app and play a video directly (set data to "v=HvncJgJbqOc")|
|%prefix%/app/stop|appId|Stops the app by the given **appId**.|
|%prefix%/app/status|appId|Get the the app status by the given **appId**.|
|%prefix%/app/installed|-|Get a list of all installed apps.|

All actions will send an answer to the corresponding result topic. The result topic is the incoming topic plus "result".
For example: The result of **%prefix%/info** will send to **%prefix%/info/result**