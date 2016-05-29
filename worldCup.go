package main

import (
	"fmt"
	"github.com/garyburd/redigo/redis"
	"encoding/json"
	"net/http"
	"io/ioutil"
	"strconv"
	"os"
	"os/exec"
	"strings"
)

// Struct used to store the API data
type fifaAPIData struct {
	Data struct {
		Group []groupData `json:group`
	} `json:data`
}

// Struct for group data
type groupData struct {
	N_MatchID int `json:n_MatchID`
	B_Finished bool `json:b_Finished`
	B_Live bool `json:b_live`
	C_Date string `json:c_Date`
	C_City string `json:c_City`
	C_CountryShort string `json:c_CountryShort`
	C_Phase_en string `json:c_Phase_en`
	C_HomeNatioShort string `json:c_HomeNatioShort`
	C_AwayNatioShort string `json:c_AwayNatioShort`
	C_HomeLogoImage string `json:c_HomeLogoImage`
	C_AwayLogoImage string `json:c_AwayLogoImage`
	N_HomeGoals int `json:n_HomeGoals`
	N_AwayGoals int `json:n_AwayGoals`
	C_Stadium string `json:c_Stadium`
}

func main () {
	fmt.Println("-------------------------------------------------------------------------------------")
	redisConn := connectToRedis()
	writeToRedis(redisConn)
	printMatches(redisConn)
	fmt.Println("-------------------------------------------------------------------------------------")
}

// Connect to local Redis
func connectToRedis()(redis.Conn){
	redisConn, err := redis.Dial("tcp", ":6379")
	if err != nil {fmt.Println("ERROR: Cannot connect to Redis")}
	return redisConn
}


// Function used to write the API JSON data to Redis
func writeToRedis(redisConn redis.Conn) {
	fifaAPIResponse := cURLEndpoint()
	fifaData := &fifaAPIData{}
	json.Unmarshal([]byte(fifaAPIResponse),&fifaData)
	for _,match := range fifaData.Data.Group {
		matchID := strconv.Itoa(match.N_MatchID)
		if matchID == os.Args[1] {
			redis.Strings(redisConn.Do("SADD","matches",match.N_MatchID))
			redis.Strings(redisConn.Do("HMSET","match:"+matchID,"Date",match.C_Date,"City",match.C_City,"Country",match.C_CountryShort,"Home",match.C_HomeNatioShort,"Away",match.C_AwayNatioShort,"HomeScore",match.N_HomeGoals,"AwayScore",match.N_AwayGoals,"Finished",match.B_Finished,"Stadium",match.C_Stadium))
		}
	}
}

// Function to cURL the FIFA API. Returns the response in JSON
func cURLEndpoint() (string) {
	req, err := http.NewRequest("GET","http://live.mobileapp.fifa.com/api/wc/matches", nil)
	if err != nil {panic(err)}
	client := http.Client{}
	res, err := client.Do(req)
	if err != nil {panic(err)}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {panic(err)}
	res.Body.Close()
	return string(body)
}

// Gets the matches from Redis and outputs the match data to screen
func printMatches(redisConn redis.Conn) {

	// Get the matches from Redis
	matches, err := redis.Strings(redisConn.Do("SMEMBERS", "matches"))
	if err != nil {fmt.Println("ERROR: Cannot get data from matches SET")}

	// Iterate over the matches found in the matches SET
	for _,match := range matches {
		// Since we may have lots of matches in Redis, only show what we want
		if match == os.Args[1] {
			// Get the match data from Redis
			matchData,err:= redis.Strings(redisConn.Do("HGETALL","match:"+ match))
			if err != nil {fmt.Println("ERROR: Cannot get data from match",match)}

			// Print the match data
			fmt.Println("Match:",match)
			fmt.Println(matchData[17])
			fmt.Println(matchData[3],",","Brasil")
			fmt.Println("-------------------------------------------------------------------------------------")

			// Print the flags
			homeFlag := "images/"+matchData[7]+".jpg"
			awayFlag := "images/"+matchData[9]+".jpg"
			fmt.Println(mergeFlags(homeFlag,awayFlag))
			fmt.Println("-------------------------------------------------------------------------------------")

			// Print the score
			homeTeam := matchData[7]+": "+matchData[11]
			awayTeam := matchData[9]+": "+matchData[13]
			fmt.Println(homeTeam)
			fmt.Println(awayTeam)
			fmt.Println("-------------------------------------------------------------------------------------","\n")

			// Print the betting info
			getWagers(redisConn,match,matchData[7],matchData[9])
		}
	}
}

// Uses jp2a to print the country flag
func printImage(image string,size string)(string) {
	v,err := exec.Command("jp2a",image,"--color","--width="+size).Output()
	if err != nil {
	 return ""
	}
	return string(v)
}

// Take the two country flags and merge them into one formatted image with vs.
func mergeFlags(awayFlag string,homeFlag string) (string) {
	f1 := strings.Split(printImage(awayFlag,"40"),"\n")
	f2 := strings.Split(printImage(homeFlag,"40"),"\n")
	for k,_ := range f1 {
		var sep string
		if k == len(f1)-2{
			sep = " vs. "
		} else {
			sep = "     "
		}
		f1[k] = fmt.Sprintf("%s%s%s",f1[k],sep,f2[k])
	}
	v := strings.Join(f1,"\n")

	return v
}

// Function used to return a nicely formatted Redis HASH
func Map(do_result interface{}, err error) (map[string] string, error){
	result := make(map[string] string, 0)
	a, err := redis.Values(do_result, err)
	if err != nil {
		return result, err
	}
	for len(a) > 0 {
		var key string
		var value string
		a, err = redis.Scan(a, &key, &value)
		if err != nil {
			return result, err
		}
		result[key] = value
	}
	return result, nil
}

// Get some waging information from Redis when things get interesting...
func getWagers(redisConn redis.Conn,matchID string,homeName string,awayName string) {

	// Set some vars
	var totalFunds int
	var homeTotal int
	var awayTotal int

	// Check if there are any bets
	numHomeBets,err := redis.Int(redisConn.Do("HLEN","bets:"+matchID+":"+homeName))
	if err != nil {panic(err)}
	numAwayBets,err := redis.Int(redisConn.Do("HLEN","bets:"+matchID+":"+awayName))
	if err != nil {panic(err)}

	// If there are bets for the HOME team print them out
	if numHomeBets > 0 {
		// Get HOME team betting info
		homeBets,err := Map(redisConn.Do("HGETALL","bets:"+matchID+":"+homeName))
		if err != nil {panic(err)}
		for _,bet := range homeBets {
			i,err := strconv.Atoi(bet)
			if err != nil {panic(err)}
			homeTotal += i
		}

		// Print HOME team betting info
		fmt.Println("-------------------------------------------------------------------------------------")
		fmt.Println(homeName,"BETS",homeTotal)
		fmt.Println("-------------------------------------------------------------------------------------")
		for k,bet := range homeBets {
			fmt.Println("-",k,"$"+bet)
		}
		fmt.Println(" ")
	} else {fmt.Println("- NO",homeName,"BETS")}

	// If there are bets for the AWAY team prin them out
	if numAwayBets > 0 {
		// Get AWAY team betting info
		awayBets,err := Map(redisConn.Do("HGETALL","bets:"+matchID+":"+awayName))
		if err != nil {panic(err)}
		for _,bet := range awayBets {
			i,err := strconv.Atoi(bet)
			if err != nil {panic(err)}
			awayTotal += i
		}

		// Print AWAY team betting info
		fmt.Println("-------------------------------------------------------------------------------------")
		fmt.Println(awayName,"BETS",awayTotal)
		fmt.Println("-------------------------------------------------------------------------------------")
		for k,bet := range awayBets{
			fmt.Println("-",k,"$"+bet)
		}
		fmt.Println(" ")
	} else {fmt.Println("- NO",awayName,"TEAM BETS")}

	// Print out the earings info
	if numHomeBets > 0 && numAwayBets > 0 {
		//Get the payouts
		totalFunds = homeTotal + awayTotal
		homePayout := totalFunds/numHomeBets
		awayPayout := totalFunds/numAwayBets

		// Print the payouts
		fmt.Println("-------------------------------------------------------------------------------------")
		fmt.Println("PAYOUT INFO")
		fmt.Println("-------------------------------------------------------------------------------------")
		fmt.Println(homeName,":",homePayout)
		fmt.Println(awayName,":",awayPayout)
	} else {fmt.Println("- NO PAYOUT INFO","\n")}
}
