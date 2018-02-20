/* This file will contain functionalities to host the main service
in which the client will make requests to. Use net/http to create
API endpoints to access the following functionalities:
	- UpdateBloomFilter
		- AddToBloomFilter
		- RepopulateBloomFilter
	- GetArrayOfUnsubscribedEmails

	- when the http endpoint is hit go to the graphana to graph those metrics
	- use docker to integrate with AWS?
	- graphana and graphite have docker images
	- mysql database is currently local buttttt, should spin up a socker container with mysql
		- so we're all on the same page
*/
package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/spf13/viper"
	"github.com/vlam321/Inf191BloomFilter/bloomManager"
	"github.com/vlam321/Inf191BloomFilter/databaseAccessObj"

	metrics "github.com/rcrowley/go-metrics"
)

type Payload struct {
	UserId int
	Emails []string
}

//The bloom filter for this server
var bf *bloomManager.BloomFilter
var shard int

/*
//handleUpdate will update the respective bloomFilter
//server will keep track of when the last updated time is. to call
//update every _ time.
func handleUpdate(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Received request: %v %v %v\n", r.Method, r.URL, r.Proto)
	bf.RepopulateBloomFilter(viper.GetInt(bfIP))
}
*/

// handleMetric records metrics (temporary method?)
func handleMetric(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Printf("1")
		return
	}
	u := r.Form["user"]
	if len(u) == 0 {
		log.Printf("2")
		return
	}

	uid, err := strconv.Atoi(u[0])
	if err != nil {
		log.Printf("3")
		return
	}
	metrics.GetOrRegisterGauge("userid.gauge", nil).Update(int64(uid))
	metrics.GetOrRegisterCounter("userid.counter", nil).Inc(1)
	if err != nil {
		log.Printf("5")
		return
	}

	log.Printf("user id  = %d\n", uid)
}

//handleFilterUnsubscripe handles when the client requests to recieve
//those emails that are unsubscribed therefore, IN the database.
func handleFilterUnsubscribed(w http.ResponseWriter, r *http.Request) {
	var buff []byte
	var payload Payload

	//Result struct made to carry the result of unsuscribed emails
	type Result struct {
		Trues []string
	}

	// log.Printf("%v", r.Body)
	//decode byets from request body
	err := json.NewDecoder(r.Body).Decode(&buff)

	//check for error; if error write 404
	if err != nil {
		w.WriteHeader(404)
		w.Write([]byte("404 - " + http.StatusText(404)))
		// http.Error(w, err.Error(), 400)
		// return
	}

	// converts the decoded result to a Payload struct
	err = json.Unmarshal(buff, &payload)
	if err != nil {
		log.Printf("error unmashaling payload %v %s\n", err, string(buff))
		return
	}
	/*
		var emailInputs []string
		for i := range payload.Emails {
			emailInputs = append(emailInputs, strconv.Itoa(payload.UserId)+"_"+payload.Emails[i])
		}
	*/

	//uses bloomManager to get the result of unsubscribed emails
	//puts them in struct, result
	emails := bf.GetArrayOfUnsubscribedEmails(map[int][]string{payload.UserId: payload.Emails})
	// filteredEmails := Result{emails}

	//convert result to json
	js, err := json.Marshal(emails)
	if err != nil {
		log.Printf("Error marshaling filteredEmails: %v\n", err)
		return
	}

	//write back to client
	w.Write(js)
}

func handleTFilterUnsubscribed(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("Received request: %v %v %v\n", r.Method, r.URL, r.Proto)
	bytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error: Unable to read request data. %v\n", err.Error())
		return
	}

	var pyld Payload
	err = json.Unmarshal(bytes, &pyld)
	if err != nil {
		log.Printf("Error: Unable to unmarshal Payload. %v\n", err.Error())
		return
	}

	//uses bloomManager to get the result of unsubscribed emails
	//puts them in struct, result
	filteredResults := bf.GetArrayOfUnsubscribedEmails(map[int][]string{pyld.UserId: pyld.Emails})

	jsn, err := json.Marshal(filteredResults)
	if err != nil {
		log.Printf("Error marshaling filtered emails. %v\n", err.Error())
		return
	}

	metrics.GetOrRegisterCounter("request.numreq", nil).Inc(1)
	//write back to client
	w.Write(jsn)
}

//updateBloomFilterBackground manages the periodic updates of the bloom
//filter. Update calls repopulate, creating a new updated bloom filter
func updateBloomFilterBackground(dao *databaseAccessObj.Conn) {
	//Set new ticker to every 2 seconds
	ticker := time.NewTicker(time.Second * 3)

	for t := range ticker.C {
		//Call update bloom filter
		metrics.GetOrRegisterGauge("dbsize.gauge", nil).Update(int64(dao.GetTableSize(0)))
		bf.RepopulateBloomFilter(shard)
		fmt.Println("Bloom Filter Updated at: ", t.Format("2006-01-02 3:4:5 PM"))
	}
}

// setBloomFilter initialize bloom filter
func setBloomFilter(dao *databaseAccessObj.Conn) {
	numEmails := uint(dao.GetTableSize(shard))
	falsePositiveProb := float64(0.001)
	bf = bloomManager.New(numEmails, falsePositiveProb)
}

// Retrieve the IPv4 address of the current AWS EC2 instance
func getMyIP() (string, error) {
	resp, err := http.Get("http://checkip.amazonaws.com/")
	if err != nil {
		return "x.x.x.x", err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)

	if err != nil {
		return "x.x.x.x", err
	}
	return string(body[:]), nil
}

func mapBf2Shard() error {
	viper.SetConfigName("bfShardConf")
	viper.AddConfigPath("settings")
	err := viper.ReadInConfig()
	if err != nil {
		return err
	}
	return nil
}

func main() {
	bfIP, err := getMyIP()
	if err != nil {
		log.Printf("BloomFilter: %v\n", err.Error())
		return
	}

	err = mapBf2Shard()
	if err != nil {
		log.Printf("BloomFilter: %v\n", err.Error())
		return
	}

	if viper.GetString("host") == "docker" {
		tabnum, err := strconv.Atoi(os.Getenv("SHARD"))
		if err != nil {
			log.Printf("Bloom Filter: %v\n", err.Error())
		}
		shard = tabnum
	} else if viper.GetString("host") == "ec2" {
		shard = viper.GetInt(bfIP)
	} else {
		log.Printf("BloomFilter: Invalid host config.")
		return
	}

	log.Printf("SUCCESSFULLY MAPPED FILTER TO DB SHARD.\n")
	log.Printf("HOSTING ON: %s\n", viper.GetString("host"))
	log.Printf("USING SHARD: %d\n", shard)

	// addr, _ := net.ResolveTCPAddr("tcp", "192.168.99.100:2003")
	// go graphite.Graphite(metrics.DefaultRegistry, 10e9, "metrics", addr)

	// bf.RepopulateBloomFilter()
	dao := databaseAccessObj.New()

	setBloomFilter(dao)

	//Run go routine to make periodic updates
	//Runs until the server is stopped
	go updateBloomFilterBackground(dao)

	// http.HandleFunc("/filterUnsubscribed", handleFilterUnsubscribed)
	// http.HandleFunc("/update", handleUpdate)
	http.HandleFunc("/metric", handleMetric)
	http.HandleFunc("/filterUnsubscribed", handleTFilterUnsubscribed)
	http.ListenAndServe(":9090", nil)
	dao.CloseConnection()
}
