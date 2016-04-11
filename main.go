package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

type Appliances map[string]Cookbooks

type Appliance struct {
	Name     string                 `json:"name"`
	Version  string                 `json:"version"`
	Roles    map[string]interface{} `json:"roles"`
	Steps    []Step                 `json:"steps"`
	AppSteps []Step                 `json:"app_steps"`
	Options  map[string]interface{} `json:"options"`
	Metadata map[string]interface{} `json:"metadata"`
}

type Step struct {
	Role    string `json:"role"`
	Package string `json:"package"`
}

type Package struct {
	Name     string                 `json:"name"`
	Version  string                 `json:"version"`
	Command  string                 `json:"command"`
	OS       string                 `json:"os"`
	Metadata map[string]interface{} `json:"metadata"`
}

type Cookbook struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type Cookbooks struct {
	UCloud     []Cookbook `json:"ucloud"`
	ThirdParty []Cookbook `json:"third_party"`
}

type Environment struct {
	Name                string                 `json:"name"`
	Description         string                 `json:"description"`
	CookbookVersions    map[string]string      `json:"cookbook_versions"`
	JSONClass           string                 `json:"json_class"`
	ChefType            string                 `json:"chef_type"`
	DefaultAttributes   map[string]interface{} `json:"default_attributes"`
	OverridesAttributes map[string]interface{} `json:"override_attributes"`
}

// {
//   "name": "spade_rabbitmq_mongodb_v5",
//   "description": "The spade rabbitmq-mongodb appliance environment",
//   "cookbook_versions": {
//     "spade_mongodb": "~> 0.10.0",
//     "spade_rabbitmq": "~> 0.4.0",
//     "ultimate_mongodb": "~> 4.2.0"
//   },
//   "json_class": "Chef::Environment",
//   "chef_type": "environment",
//   "default_attributes": {
//     "ossec": {
//       "config": {
//         "enable_email": false
//       }
//     }
//   },
//   "override_attributes": {
//   }
// }

var (
	applianceFlag  = flag.String("appl", "", "AMP Appliance name")
	ampEndpoint    = "http://amp-prod.mia.ucloud.int:5001"
	knifeFile      = "/Users/patrickr/hoth-deployment-orchestration/infrastructure/knife-paas.rb"
	commandRegex   = regexp.MustCompile(`-o role\[(.*)\] -E (.*)`)
	chefDirectory  = "/Users/patrickr/hoth-deployment-orchestration/infrastructure"
	ucloudPrefixes = []string{"uc", "ult", "ultimate", "spade", "hardening", "logging", "monitoring"}
)

func main() {
	flag.Parse()

	appliance := getAppliance(*applianceFlag)
	jsonPrettyPrint(os.Stdout, appliance)

	appliances := make(map[string]Cookbooks, 0)
	for _, step := range appliance.Steps { // TODO: loop over appl_steps as well
		pkg := getPackage(step.Package)
		jsonPrettyPrint(os.Stdout, pkg)

		role, env := parseRoleAndEnv(pkg.Command)
		output := knife("solve", "role["+role+"]", "-E", env, "-c", knifeFile)
		log.Println(string(output))
		cookbooks := parseCookbooks(output)
		appliances[appliance.Name] = *cookbooks
		jsonPrettyPrint(os.Stdout, cookbooks)
	}

	http.HandleFunc("/appliances", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		jsonPrettyPrint(w, appliances)
	})

	http.ListenAndServe("0.0.0.0:5002", nil)
}

func parseCookbooks(data []byte) *Cookbooks {
	cookbooks := Cookbooks{make([]Cookbook, 0), make([]Cookbook, 0)}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines[1 : len(lines)-1] {
		cols := strings.Split(line, " ")
		cookbook := Cookbook{Name: cols[0], Version: cols[1]}
		var ucloudOwned bool
		for _, prefix := range ucloudPrefixes {
			if strings.HasPrefix(cookbook.Name, prefix) {
				ucloudOwned = true
				break
			}
		}

		if ucloudOwned {
			cookbooks.UCloud = append(cookbooks.UCloud, cookbook)
		} else {
			cookbooks.ThirdParty = append(cookbooks.ThirdParty, cookbook)
		}
	}

	return &cookbooks
}

func knife(cmd string, args ...string) []byte {
	pwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("knife: %v", err)
	}
	if err := os.Chdir(chefDirectory); err != nil {
		log.Fatalf("knife: %v", err)
	}
	defer os.Chdir(pwd)

	args = append([]string{cmd}, args...)
	output, err := exec.Command("knife", args...).Output()
	if err != nil {
		log.Fatalf("knife: %v", err)
	}

	return output
}

func getAppliance(name string) *Appliance {
	log.Printf("Querying AMP for appliance '%s' ...", name)
	applianceURI := ampEndpoint + "/appliances/" + name
	resp, err := http.Get(applianceURI)
	if err != nil {
		log.Fatalf("GET %s : %v", applianceURI, err)
	}
	defer resp.Body.Close()

	var appliance Appliance
	if err := json.NewDecoder(resp.Body).Decode(&appliance); err != nil {
		log.Fatalf("JSON decode: %v", err)
	}

	return &appliance
}

func getPackage(name string) *Package {
	log.Printf("Querying AMP for package '%s' ...", name)
	packageURI := ampEndpoint + "/packages/" + name
	resp, err := http.Get(packageURI)
	if err != nil {
		log.Fatalf("GET %s : %v", packageURI, err)
	}
	defer resp.Body.Close()

	var pkg Package
	if err := json.NewDecoder(resp.Body).Decode(&pkg); err != nil {
		log.Fatalf("JSON decode: %v", err)
	}

	return &pkg
}

func parseRoleAndEnv(command string) (string, string) {
	matches := commandRegex.FindStringSubmatch(command)
	if len(matches) != 3 {
		log.Fatalf("parseRoleAndEnv could not parse role and environment from: %s", command)
	}
	return matches[1], matches[2]
}

func jsonPrettyPrint(w io.Writer, v interface{}) {
	data, err := json.MarshalIndent(v, " ", "  ")
	if err != nil {
		log.Fatalf("jsonPrettyPrint: %v", err)
	}
	fmt.Fprintf(w, "%v\n\n", string(data))
}
