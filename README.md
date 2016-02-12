# GPUManager
A tool to handle creation and distribution of ProtoMol simulations.

# Running
1. Get package dependancies `go get -u ./...`
2. Install [Goose](https://bitbucket.org/liamstask/goose/) `go get -u bitbucket.org/liamstask/goose/cmd/goose`
3. Move into the assets directory `cd assets`
4. Run the database migrations `goose up`
5. Start the server `go run ../*.go`
