package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"reflect"
	"time"

	msk_protobuf "msk-pub/protobuf"
	"msk-pub/types"

	"github.com/joho/godotenv"
	"github.com/nats-io/nats.go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func main() {
	collectionName := "testing"
	fetchJSONFile := "fetch_shorter.json"
	insertNewChannel := "channels.insertNewChannel"
	insertUpdateChannel := "channels.insertUpdateChannel"
	natsServer := "nats://localhost:4222"

	// Loads the .env file
	godotenv.Load()

	// Setup connection with MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Connect to database
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(os.Getenv("MONGODB_URI")))
	if err != nil {
		log.Fatal(err)
	}
	mskCollection := client.Database("msk").Collection(collectionName)

	// Read JSON file with other data
	newData, err := ioutil.ReadFile(fetchJSONFile)
	if err != nil {
		log.Fatal(err)
	}

	// Read JSON into Go format
	var newJSONFile types.FetchJSON
	err = json.Unmarshal(newData, &newJSONFile)
	if err != nil {
		log.Fatal("Error during Unmarshal(): ", err)
	}

	newSamples := newJSONFile.Results
	opts := options.FindOne().SetSort(bson.M{"last_modified": -1})

	// Create a connection to a nats-server:
	nc, err := nats.Connect(natsServer)
	if err != nil {
		log.Fatalln(err)
	}

	// Loop through each sample and insert/update the database
	for i := range newSamples {
		newSample := newSamples[i]
		dmp_sample_id := newSample.Meta_data.Dmp_sample_id
		filter := bson.M{"meta_data.dmp_sample_id": dmp_sample_id}

		// Search database for existing sample using dmp_sample_id
		var oldSample types.Result
		err = mskCollection.FindOne(ctx, filter, opts).Decode(&oldSample)
		if err != nil {

			// If no sample exists with dmp_sample_id, then insert new document
			if err == mongo.ErrNoDocuments {
				fmt.Printf("No document with dmp_sample_id %s found; inserting new document\n", dmp_sample_id)
				insertDocument(mskCollection, ctx, newSample)
				publishMessage(newSample, nc, insertNewChannel)

			} else {
				log.Fatal(err)
			}
		} else { // Only insert new document if different from most recent existing document
			oldSample.Last_modified = nil

			// Sample is different from most recent existing document; insert new
			if !reflect.DeepEqual(newSample, oldSample) {
				fmt.Printf("Document with dmp_sample_id %s found but is different; inserting new version\n", dmp_sample_id)
				insertDocument(mskCollection, ctx, newSample)
				publishMessage(newSample, nc, insertUpdateChannel)

			} else { // Sample is the same as most recent existing document; skip
				fmt.Printf("Document with dmp_sample_id %s is the same; skipping\n", dmp_sample_id)
				publishMessage(newSample, nc, insertNewChannel)

			}
		}
	}

	nc.Drain()
}

// Function for publishing a message through the NATS server
func publishMessage(newSample types.Result, nc *nats.Conn, channel string) {

	// Result struct -> JSON
	newSampleBytes, err := json.Marshal(newSample)
	if err != nil {
		log.Fatal(err)
	}

	// JSON -> ProtoMessage
	protoJSON := &msk_protobuf.Result{}
	err = protojson.Unmarshal(newSampleBytes, protoJSON)
	if err != nil {
		log.Fatal(err)
	}

	// ProtoMessage -> []byte
	protoData, err := proto.Marshal(protoJSON)
	if err != nil {
		log.Fatal(err)
	}

	// Send bytes through channel
	nc.Publish(channel, protoData)
	fmt.Println("Sent!")
}

// Function for inserting a document to the database
func insertDocument(mskCollection *mongo.Collection, ctx context.Context, newSampleData types.Result) {

	// Add last_modified field to the current time
	tm := time.Now()
	newSampleData.Last_modified = primitive.NewDateTimeFromTime(tm)

	// Insert document into databse
	res, err := mskCollection.InsertOne(ctx, newSampleData)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("inserted document with ID %v\n", res.InsertedID)
}