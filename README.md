# Loki BT Service - Bluetooth for the Android emulator

Loki BT allows Android developers to use Bluetooth in the Android emulator that ships with Android Studio by emulating Bluetooth over TCP/IP. Loki consists of a web-service and an Android library. This is the repo of the web-service, which is licensed as open-source under the AGPL version 3.

*Loki BT is still an alpha! Its core functionality is working, but there are still some parts of Android's Bluetooth API missing. Apart from that there could still be undiscovered bugs and breaking interface changes are likely to happen, so it is not recommended to use it in production projects, yet.*

## First run

To run or compile the Loki BT service the [Go develeopment framework](https://golang.org/) needs to be installed on your system.

Once this is installed, you can clone and run the Loki BT service easily with the following commands:

```
git clone --branch=next https://github.com/lokibt/service.git
cd service
go run main.go --debug
```

Don't forget to [make the necessary changes to connect to an own Loki BT service](#connect-to-a-local-loki-bt-service) in the code of your app, before testing the service.

However, to run the Loki BT service in this way is only recommended for testing purposes.

## Installation

You can clone, compile and install the Loki BT service easily with the following commands:

```
git clone https://github.com/lokibt/service.git
cd service
go build -o lokibt
sudo cp lokibt /usr/local/bin
```

The installation path */usr/local/bin* is only an example, of course. You can install the binary to any place you want.

Windows user should use `go build -o lokibt.exe` to create a binary with the proper file-extension.

## Usage

Starting the Loki BT service is super simple:

```
/usr/local/bin/lokibt
```

### Connect to a local Loki BT service

The Loki BT library connects to the official web-service by default. If you want to use you your own Loki BT service installation, you have to add some parameters to the Intent to start Bluetooth:

```
Intent intent = new Intent(BluetoothAdapter.ACTION_REQUEST_ENABLE);
// The following two lines tell the Loki BT library to connect to a local Loki BT service
intent.putExtra(BluetoothAdapter.EXTRA_LOKIBT_HOST, "10.0.2.2");
intent.putExtra(BluetoothAdapter.EXTRA_LOKIBT_PORT, 8198);
startActivityForResult(intent, REQUEST_ENABLE);
```

Keep in mind that the `BluetoothAdapter.ACTION_REQUEST_DISCOVERABLE` intent could also start Bluetooth:

```
Intent intent = new Intent(BluetoothAdapter.ACTION_REQUEST_DISCOVERABLE);
intent.putExtra(BluetoothAdapter.EXTRA_DISCOVERABLE_DURATION, 600);
// The following two lines tell the Loki BT library to connect to a local Loki BT service
intent.putExtra(BluetoothAdapter.EXTRA_LOKIBT_HOST, "10.0.2.2");
intent.putExtra(BluetoothAdapter.EXTRA_LOKIBT_PORT, 8198);
startActivityForResult(intent, REQUEST_DISCOVERABLE);
```

If your Service does not run on your local machine, you just have to change the `BluetoothAdapter.EXTRA_LOKIBT_HOST` value to the appropriate address.

### Command-line arguments

* `--debug`: Be very verbose and log extended information for debugging.

----

Copyright 2020 Torben Haase \<[https://pixelsvsbytes.com](https://pixelsvsbytes.com)>
