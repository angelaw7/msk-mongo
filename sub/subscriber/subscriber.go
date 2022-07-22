package subscriber

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	msk_protobuf "msk-sub/protobuf"
	"os"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
)

func SubscriberMain(masterJSONFile string, channel string, natsServer string) {

	allChannels := "channels.*"

	// Check that the file exists
	if channel == allChannels {
		err := checkFile(masterJSONFile)
		if err != nil {
			log.Fatal(err)
		}
	}

	// Set up connection to NATS serverv
	var nc *nats.Conn
	var err error
	wait := make(chan bool)
	for {
		nc, err = nats.Connect(natsServer)
		if err != nil {
			fmt.Println("Attempting to connect to server")
		} else {
			break
		}
	}

	// Create hashmap for the sample IDs that have been inserted into master JSON
	idMap := map[string]int{}

	// Subscribe to channel
	nc.Subscribe(channel, func(m *nats.Msg) {

		// Read amd print the data
		newStruct := &msk_protobuf.Result{}
		err = proto.Unmarshal(m.Data, newStruct)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(newStruct)

		// Write to master JSON if sub is channels.*
		if channel == allChannels {

			// Read master JSON
			file, err := ioutil.ReadFile(masterJSONFile)
			if err != nil {
				log.Fatal(err)
			}
			data := []msk_protobuf.Result{}
			json.Unmarshal(file, &data)

			// Sample data ID and check if in map
			dmpID := newStruct.MetaData.DmpSampleId
			_, check := idMap[dmpID]

			// Check whether another version of the sample is already in master JSON file
			if check {
				fmt.Println("Another version of this sample was uploaded already in current JSON; rewriting old one...")
				indexOfExistingSample := idMap[dmpID]
				data = append(data[:indexOfExistingSample], data[indexOfExistingSample+1:]...)
			}
			data = append(data, *newStruct)
			idMap[dmpID] = len(data) - 1

			// Write the new sample data into the master JSON file
			dataBytes, err := json.Marshal(data)
			if err != nil {
				log.Fatal(err)
			}
			err = ioutil.WriteFile(masterJSONFile, dataBytes, 0644)
			if err != nil {
				log.Fatal(err)
			}
		}

	})

	fmt.Println("Subscribed to", channel)
	<-wait
}

// Function to check that the master JSON file name exits;
// Deletes contents if it exits and creates new file if not
func checkFile(filename string) error {
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		_, err := os.Create(filename)
		if err != nil {
			log.Fatalln(err)
		}
	} else {
		err := os.Truncate(filename, 0)
		if err != nil {
			log.Fatalln(err)
		}
	}
	return nil
}