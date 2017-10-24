package main

import (
    "flag"
    "net/http"
    "encoding/json"
    "log"
    "time"
    "os"
    "strconv"
    "io"
 
    "github.com/cloudfoundry-community/go-cfclient"    
)

var (
	ApiAddress = flag.String(
		"api.address", "",
		"Cloud Foundry API Address ($API_ADDRESS).",
	)
        
	ClientID = flag.String(
                "api.id", "", 
                "Cloud Foundry UAA ClientID ($CF_CLIENT_ID).",
        )

	ClientSecret = flag.String(
                "api.secret", "", 
                "Cloud Foundry UAA ClientSecret ($CF_CLIENT_SECRET).",
        )

	SkipSSL = flag.Bool(
                "skip.ssl", false, 
                "Disalbe SSL validation ($SKIP_SSL).",
        )
	
	Frequency = flag.Uint(
                "update.frequency", 3,
                "SD Update frequency in minutes ($FREQUENCY).",
        )
)

var request chan chan []AppInfo
var mapchan chan []AppInfo


type AppInfo struct {
        Name  string `json:"name"`
        Guid  string `json:"guid"`
        Space string `json:"space"`
        Org   string `json:"org"`
}

type SpaceInfo struct {
	Name             string `json:"name"`
        OrgName		 string `json:"orgname"`
}

func overrideFlagsWithEnvVars() {
	overrideWithEnvVar("API_ADDRESS", ApiAddress)
	overrideWithEnvVar("CF_CLIENT_ID", ClientID)
	overrideWithEnvVar("CF_CLIENT_SECRET", ClientSecret)
	overrideWithEnvBool("SKIP_SSL", SkipSSL)
	overrideWithEnvUint("FREQUENCY", Frequency)
}

func overrideWithEnvVar(name string, value *string) {
	envValue := os.Getenv(name)
	if envValue != "" {
		*value = envValue
	}
}

func overrideWithEnvUint(name string, value *uint) {
	envValue := os.Getenv(name)
	if envValue != "" {
		intValue, err := strconv.Atoi(envValue)
		if err != nil {
			log.Fatalln("Invalid `%s`: %s", name, err)
		}
		*value = uint(intValue)
	}
}

func overrideWithEnvBool(name string, value *bool) {
	envValue := os.Getenv(name)
	if envValue != "" {
		var err error
		*value, err = strconv.ParseBool(envValue)
		if err != nil {
			log.Fatalf("Invalid `%s`: %s", name, err)
		}
	}
}

func UpdateAppMap (client *cfclient.Client) {
   appmap := make([]AppInfo,0)

   go GenAppMap(client)  

   c := time.Tick(time.Duration(*Frequency) * time.Minute)
   for {
     select {
     case ch := <- request:
        ch <- appmap 
     case <-c:
	go GenAppMap(client)
     case freshmap := <- mapchan:
	appmap = freshmap 
	log.Printf("appmap updated")
     }
   }
}

func GenAppMap(client *cfclient.Client) {
	log.Println("generating fresh app map")

	apps,err := client.ListAppsByQuery(nil)
	if err != nil {
		log.Printf("Error generating list of apps from CF: %v", err)
	}  

	orgs,err := client.ListOrgs()
        if err != nil {
                log.Printf("Error generating list of orgs from CF: %v", err)
        }

	// create map of org guid to org name
	orgmap := map[string]string{}
	for _, org := range orgs {
		orgmap[org.Guid] = org.Name
	} 	

        spaces,err := client.ListSpaces()
        if err != nil {
                log.Printf("Error generating list of spaces from CF: %v", err)
        }

	// create a map of space guid to space name and org name	
	spacemap := map[string]SpaceInfo{}
	for _, space := range spaces {
		spacemap[space.Guid] = SpaceInfo { Name:space.Name , OrgName: orgmap[space.OrganizationGuid] }
	}  

 	tempmap := make([]AppInfo,0)
 	var tempapp AppInfo 
 	for _, app := range apps {
		tempapp.Name=app.Name
		tempapp.Guid=app.Guid
		tempapp.Space=spacemap[app.SpaceGuid].Name
		tempapp.Org=spacemap[app.SpaceGuid].OrgName
     
		tempmap = append(tempmap, tempapp)
	}
	mapchan <- tempmap
}

func main() {
  flag.Parse()
  overrideFlagsWithEnvVars()
 
  var port string
  request = make(chan chan []AppInfo)
  mapchan = make(chan []AppInfo)

  c := &cfclient.Config{
    ApiAddress:        *ApiAddress,
    ClientID:          *ClientID,
    ClientSecret:      *ClientSecret,
    SkipSslValidation: *SkipSSL,
  }
  client, err := cfclient.NewClient(c)
  if err != nil {
	log.Fatal("Error connecting to API: %s", err.Error())
	os.Exit(1)
  }

 go UpdateAppMap(client)

 if port = os.Getenv("PORT"); len(port) == 0 {
        port = "8080" 
 }

 http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "Cloud Foundry application lister.  Actual Information is served at /rest/apps/")
    })

 http.HandleFunc("/rest/apps/", func(w http.ResponseWriter, r *http.Request) {
        response := make(chan []AppInfo)
        request <- response
	appmapCopy := <- response
        json.NewEncoder(w).Encode(appmapCopy)
    })
 log.Fatal(http.ListenAndServe(":" + port, nil))
}
