# lndRouting

The lndRouting deamon in an unstable implementation of a lightning distributed routing node. If fully conforms to the specification of the upcoming lightning distributed routing protocol and is, in the current state capable of:

## Installation


## Usage

```
./lndRouting -<option>=<VALUE>
```

The available options are:

```
bitcoinRPCUser=<Bitcoin core RPC user> (required)
bitcoinRPCPassword=<Bitcoin core RPC password> (required)
bitcoinClientHost=<Bitcoin core host address> (default: localhost)
bitcoinClientPort=<Bitcoin core host port> (default: 18332)
lightningClientHost=<LND host address> (default: localhost)
lightningClientPort=<LND host port> (default: 10009)
macaroonPath=<Path to the macaroon used with LND for authenticate> (default: $HOME/.lnd/data/chain/bitcoin/mainnet/admin.macaroon)
tlsCertPath=<Path to the TLS certificate used with LND for authentication> (default: $HOME/.lnd/tls.cert)
port=<Port to listen for new connections to the routing client> (default: 8695)
dataPath=<Path to directory holding the application's data> (default: $HOME/.lndRouting/data")
```

## Contributing
Pull requests are welcome. For major changes, please open an issue first to discuss what you would like to change.

Please make sure to update tests as appropriate.

## Let's talk!

I'm always on DM away <a href="https://twitter.com/piggydeveloper" target="_blank">`@piggydeveloper`</a>.

If twitter is not your thing drop me an e-mail [here](mailto:joaosvalente@tecnico.ulisboa.pt?subject=[GitHub]%20Lightning%Distributed%20Routing).

## License
This software is released under the terms of the MIT license. For more see https://opensource.org/licenses/MIT.
