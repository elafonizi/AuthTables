package main

import (
	"encoding/json"
	"fmt"
	"gopkg.in/redis.v4"
	"io/ioutil"
	"net/http"
	"time"
)

//Main
func main() {

	//First time online, load historical data for bloom
	loadRecords()

	//Announce that we're running
	fmt.Printf("AuthTables is running.\n")
	//Open a webserver
	http.HandleFunc("/", handler)
	http.ListenAndServe(":8080", nil)

}

func getRecordHashesFromRecord(rec Record) (recordhashes RecordHashes) {

	rh := RecordHashes{
		uid:     []byte(rec.UID),
		uid_mid: []byte(rec.UID + ":" + rec.MID),
		uid_ip:  []byte(rec.UID + ":" + rec.IP),
		uid_all: []byte(rec.UID + ":" + rec.IP + ":" + rec.MID),
		ip_mid:  []byte(rec.IP + ":" + rec.MID),
		mid_ip:  []byte(rec.MID + ":" + rec.IP),
	}

	return rh
}

func check(rec Record, w http.ResponseWriter) {
	//We've received a request to /check and now
	//we need to see if it's suspicious or not.

	//Create []byte Strings for bloom
	rh := getRecordHashesFromRecord(rec)

	//These is ip:mid and mid:ip, useful for `key`
	//commands hunting for other bad guys. This May
	//be a seperate db, sharded elsewhere in the future.
	//Example: `key 1.1.1.1:*` will reveal new machine ID's
	//seen on this host.
	//This may include evil data, which is why we don't attach to a user.
	writeRecord(rh.ip_mid)
	writeRecord(rh.mid_ip)

	//Do we have it in bloom?
	//if filter.Test([]byte(r.URL.Path[1:])) {
	if filter.Test(rh.uid_all) {
		//We've seen everything about this user before. MachineID, IP, and user.
		fmt.Printf("Known user information.\n")
		fmt.Fprintln(w, "OK")
		//Write Everything.
		writeUserRecord(rh)

	} else if (filter.Test(rh.uid_mid)) || (filter.Test(rh.uid_ip)) {

		fmt.Printf("Either " + rec.IP + " or " + rec.MID + " is known. Adding both.\n")
		fmt.Fprintln(w, "OK")
		writeUserRecord(rh)

	} else if !(filter.Test(rh.uid)) {
		fmt.Printf("New user with no records. Adding records.\n")
		writeUserRecord(rh)

		fmt.Fprintln(w, "OK")
	} else {
		fmt.Printf("IP: " + rec.IP + " and MID: " + rec.MID + " are suspicious.\n")
		fmt.Fprintln(w, "BAD")
	}

}

func add(rec Record, w http.ResponseWriter) {

	//JSON record is sent to /add, we add all of it to bloom.
	rh := getRecordHashesFromRecord(rec)
	writeUserRecord(rh)
	fmt.Fprintln(w, "ADD")
}

func handler(w http.ResponseWriter, r *http.Request) {

	//Get our body from the request (which should be JSON)
	r.ParseForm()
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		fmt.Println("error:", err)
	}

	//Cast our JSON body content to prepare for Unmarshal
	client_authdata := []byte(body)

	//Decode some JSON and get it into our Record struct
	var m Record
	err = json.Unmarshal(client_authdata, &m)
	if err != nil {
		fmt.Println("error:", err)
	}

	//Which Route?
	route := r.URL.Path[1:]

	if route == "check" {
		fmt.Printf("Checking %s: ", m.UID)
		check(m, w)
	} else if route == "add" {
		fmt.Println("Adding: ", m)
		add(m, w)
	} else {
		fmt.Println("Bad Request.")

	}

}

func writeRecord(key []byte) {

	err := client.Set(string(key), 1, 0).Err()
	if err != nil {
		//(TODO Try to make new connection)
		rebuildConnection()
		fmt.Println("Record not written. Attempting to reconnect...")
		fmt.Println(err)
	}

}

func rebuildConnection() {
	fmt.Println("Attempting to reconnect...")
	client = redis.NewClient(&redis.Options{
		Addr:     c.Host + ":" + c.Port,
		Password: c.Password, // no password set
		DB:       0,          // use default DB
	})
}

func loadRecords() {
	timeTrack(time.Now(), "Loading records")

	var cursor uint64
	var n int
	for {
		var keys []string
		var err error
		keys, cursor, err = client.Scan(cursor, "", 10).Result()
		if err != nil {

			fmt.Println("Could not connect to Database. Error! Continuing without history.")
			break
		}
		n += len(keys)

		for _, element := range keys {
			filter.Add([]byte(element))
		}

		if cursor == 0 {
			break
		}
	}

	fmt.Printf("Loaded %d historical records.\n", n)
}

func writeUserRecord(rh RecordHashes) {

	err := client.MSetNX(string(rh.uid), 1, string(rh.uid_mid), 1, string(rh.uid_ip), 1, string(rh.uid_ip), 1, string(rh.uid_all), 1).Err()
	if err != nil {
		//(TODO Try to make new connection)
		fmt.Println("MSetNX failed")
		fmt.Println(err)
	}

	//Bloom
	filter.Add(rh.uid_mid)
	filter.Add(rh.uid_ip)
	filter.Add(rh.uid)
	filter.Add(rh.uid_all)
}

func timeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	fmt.Println(name + " took " + elapsed.String())
}
