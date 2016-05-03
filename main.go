package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
	"io/ioutil"
	"bytes"
)

type Appliance struct {
	Name     string                 `json:"name"`
	Version  string                 `json:"version"`
	Roles    map[string]interface{} `json:"roles"`
	Steps    []Step                 `json:"steps"`
	AppSteps []AppStep              `json:"app_steps"`
	Options  map[string]interface{} `json:"options"`
	Metadata map[string]interface{} `json:"metadata"`
}

type ApplianceInfo struct {
	OrigAppliance	Appliance		`json:"orig_appliance"`
	LockedAppliance	Appliance		`json:"locked_appliance"`
	AppStepInfos	[]AppStepInfo	`json:"app_step_infos"`
}

type Step struct {
	Role    string `json:"role"`
	Package string `json:"package"`
}

type AppStep struct {
	Role    string `json:"role"`
	Command string `json:"command"`
	Name 	string `json:"name"`
}

type AppStepInfo struct {
	OrigAppStep			AppStep					`json:"orig_app_step"`
	LockedAppStep		AppStep					`json:"locked_app_step"`
	KnifeCookbooks		Cookbooks				`json:"knife_cookbooks"`
	OrigEnvironment		Environment				`json:"orig_environment"`
	LockedEnvironment	Environment				`json:"locked_environment"`
}

type ApplianceList []ApplianceLinks

type ApplianceLinks struct {
	Name	string	`json:"name"`
	Version	string	`json:"version"`
	Links	[]Link	`json:"links"`
}

type PackageList []PackageLinks

type PackageLinks struct {
	Name	string	`json:"name"`
	Version	string	`json:"version"`
	Links	[]Link	`json:"links"`
}

type Link struct {
	HREF	string	`json:"href"`
	Rel		string	`json:"rel"`
}

type Package struct {
	Name     			string                 	`json:"name"`
	Version  			string                 	`json:"version"`
	Command  			string                 	`json:"command"`
	OS       			string                 	`json:"os"`
	Metadata 			map[string]interface{} 	`json:"metadata"`
}

type PackageInfo struct {
	OrigPackage			Package					`json:"orig_package"`
	LockedPackage		Package					`json:"locked_package"`
	KnifeCookbooks		Cookbooks				`json:"knife_cookbooks"`
	OrigEnvironment		Environment				`json:"orig_environment"`
	LockedEnvironment	Environment				`json:"locked_environment"`
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

var (
	ampEndpoint    = "http://amp-prod.mia.ucloud.int:5001"
	commandRegex   = regexp.MustCompile(`-o '?((recipe|role)\[.*\])'? *(-E)? *([a-zA-Z0-9_\-]*)?( -l debug)*`)
	chefEnvironments = "./chef_environments"
	latestAppliancesFile = "./latest_appliances_list"
	ucloudPrefixes = []string{
		"chrome",
		"dot_net",
		"firefox",
		"gallio",
		"hardening",
		"innosetup",
		"internet_explorer",
		"jmeter",
		"kms",
		"logging",
		"monitor",
		"monitoring",
		"openstack",
		"spade_elasticsearch",
		"spade_haproxy",
		"spade_mongodb",
		"spade_rabbitmq",
		"spade_redis",
		"spade_sql_server",
		"spade_sql_server_2012",
		"spade_ubuntu",
		"swagger",
		"swat-jumphost",
		"symantec",
		"teamcity",
		"teamcity-server",
		"teamcity_agent",
		"test",
		"tests",
		"typescript",
		"uc-apache2",
		"uc-common",
		"uc-elasticsearch",
		"uc-grafana",
		"uc-graphite",
		"uc-kibana",
		"uc-logagent",
		"uc-logstash",
		"uc-monit",
		"uc-monitor",
		"uc-ntpclient",
		"uc-redis",
		"uc-security",
		"uc-serverspec",
		"ucp_chef_server",
		"ult-monitor",
		"ult-rabbitmq",
		"ult-ssh",
		"ultimate_bower",
		"ultimate_docker",
		"ultimate_elasticsearch",
		"ultimate_go",
		"ultimate_grunt",
		"ultimate_haproxy",
		"ultimate_iis",
		"ultimate_java",
		"ultimate_mongodb",
		"ultimate_mongodb_enterprise",
		"ultimate_nodejs",
		"ultimate_nvm",
		"ultimate_ohai_plugins",
		"ultimate_phantomjs",
		"ultimate_python",
		"ultimate_rabbitmq",
		"ultimate_redis",
		"ultimate_samba",
		"ultimate_sql_server",
		"ultimate_swagger",
		"ultimate_ubuntu_app",
		"ultimate_windows",
		"utm_base",
		"utm_build_agent",
		"utm_sql_server",
		"visual_studio",
		"windows_patches",
	}
)

func main() {
	applianceList := getApplianceList()
//	jsonPrettyPrint(os.Stdout, applianceList)

	var latestAppliances = make(map[string]bool)
	content, err := ioutil.ReadFile(latestAppliancesFile)
	if err != nil {
		log.Fatalf("Could not read file %s: %v", latestAppliancesFile, err)
	}
	for _, appliance := range strings.Split(string(content), "\r\n") {
		latestAppliances[appliance] = true
	}
//	fmt.Printf("latest appliances: %v", latestAppliances)
//	os.Exit(-1)

	var updatedPackages = make(map[string]PackageInfo)

	os.Mkdir(chefEnvironments, 0755)

	for _, applianceLinks := range *applianceList {
		if !latestAppliances[applianceLinks.Name + "-"  + applianceLinks.Version] {
			log.Printf("Skipping appliance that is not latest version: %s", applianceLinks.Name + "-"  + applianceLinks.Version)
			continue
		}

		appliance := getAppliance(applianceLinks.Links[0].HREF)
//		jsonPrettyPrint(os.Stdout, appliance)

		applianceInfo := ApplianceInfo{OrigAppliance: *appliance, LockedAppliance: *appliance}

		var createLockedAppliance bool

		steps := make([]Step, 0)
		log.Printf("Looping through steps for appliance %s", appliance.Name + "-" + appliance.Version)
		for _, step := range appliance.Steps {
			if pkgInfo, ok := updatedPackages[step.Package]; ok {
				log.Printf("Package %s has already been updated to locked package %s", step.Package, pkgInfo.LockedPackage.Name + "-" + pkgInfo.LockedPackage.Version)
				step.Package = pkgInfo.LockedPackage.Name + "-" + pkgInfo.LockedPackage.Version
				steps = append(steps, step)
				if strings.HasPrefix(pkgInfo.LockedPackage.Name, "l-") { createLockedAppliance = true }
				continue
			}

			pkg := getPackage(step.Package)
//			jsonPrettyPrint(os.Stdout, pkg)

			pkgInfo := PackageInfo{OrigPackage: *pkg, LockedPackage: *pkg}

			if !strings.Contains(pkg.Command, "chef-client") {
				log.Printf("Skipping command without chef-client; package: %s; command: %s", pkg.Name, pkg.Command)
				steps = append(steps, step)
				updatedPackages[step.Package] = pkgInfo
				continue
			}

			runList, env := parseRunListAndEnv(pkg.Command)
//			log.Printf("RunList: %s, Env: %s", runList, env)
			var output []byte
			if env != "" {
				output = knife("solve", runList, "-E", env)
			} else {
				output = knife("solve", runList)
			}

	//		log.Println(string(output))
			cookbooks := parseCookbooks(output)
			pkgInfo.KnifeCookbooks = *cookbooks
//			jsonPrettyPrint(os.Stdout, pkgInfo)

			output = []byte{}
			if env != "" {
				output = knife("environment", "show", env, "-fj")
	//			log.Println(string(output))
				origEnvironment := parseEnvironment(output)
				pkgInfo.OrigEnvironment = *origEnvironment
				pkgInfo.LockedEnvironment = *origEnvironment
				pkgInfo.LockedEnvironment.Name = "l-" + pkg.Name + "-" + pkg.Version

				var lockedCookbookVersions = make(map[string]string)
				for _, cookbook := range pkgInfo.KnifeCookbooks.UCloud {
					lockedCookbookVersions[cookbook.Name] = "= " + cookbook.Version
				}
				for _, cookbook := range pkgInfo.KnifeCookbooks.ThirdParty {
					lockedCookbookVersions[cookbook.Name] = "= " + cookbook.Version
				}
				pkgInfo.LockedEnvironment.CookbookVersions = lockedCookbookVersions

				pkgInfo.LockedPackage.Name = "l-" + pkg.Name
				pkgInfo.LockedPackage.Command = strings.Replace(pkg.Command, "-E " + env, "-E " + pkgInfo.LockedEnvironment.Name, 1)

			} else {
				pkgInfo.LockedEnvironment.Name = "l-" + pkg.Name + "-" + pkg.Version
				pkgInfo.LockedEnvironment.JSONClass = "Chef::Environment"
				pkgInfo.LockedEnvironment.ChefType = "environment"
				pkgInfo.LockedEnvironment.DefaultAttributes = make(map[string]interface{})
				pkgInfo.LockedEnvironment.OverridesAttributes = make(map[string]interface{})

				var lockedCookbookVersions = make(map[string]string)
				for _, cookbook := range pkgInfo.KnifeCookbooks.UCloud {
					lockedCookbookVersions[cookbook.Name] = "= " + cookbook.Version
				}
				for _, cookbook := range pkgInfo.KnifeCookbooks.ThirdParty {
					lockedCookbookVersions[cookbook.Name] = "= " + cookbook.Version
				}
				pkgInfo.LockedEnvironment.CookbookVersions = lockedCookbookVersions

				pkgInfo.LockedPackage.Name = "l-" + pkg.Name
				pkgInfo.LockedPackage.Command = pkg.Command + " -E " + pkgInfo.LockedEnvironment.Name
			}

			data, err := json.MarshalIndent(pkgInfo.LockedEnvironment, " ", "  ")
			if err != nil {
				log.Fatalf("JSON Marshall: %v", err)
			}

			lockedEnvironmentFile := chefEnvironments + "/" + pkgInfo.LockedEnvironment.Name + ".json"
			if err := ioutil.WriteFile(lockedEnvironmentFile, data, 0755); err != nil {
				log.Fatalf("Failed to write to file: %v", err)
			}

			log.Printf("Creating new chef environment: %s", pkgInfo.LockedEnvironment.Name)
//			output = knife("environment", "from", "file", lockedEnvironmentFile)
//			log.Println(string(output))
//			os.Exit(-1)

			log.Printf("Creating new package %s", pkgInfo.LockedPackage.Name + "-" + pkgInfo.LockedPackage.Version)
			jsonPrettyPrint(os.Stdout, pkgInfo)
//			postPackage(&pkgInfo.LockedPackage)
//			os.Exit(-1)

			updatedPackages[step.Package] = pkgInfo
			step.Package = pkgInfo.LockedPackage.Name + "-" + pkgInfo.LockedPackage.Version
			steps = append(steps, step)
			createLockedAppliance = true
		}
		applianceInfo.LockedAppliance.Steps = steps

		appSteps := make([]AppStep, 0)
		appStepInfos := make([]AppStepInfo, 0)
		log.Printf("Looping through app steps for appliance %s", appliance.Name + "-" + appliance.Version)
		for _, appStep := range appliance.AppSteps {
			appStepInfo := AppStepInfo{OrigAppStep: appStep, LockedAppStep: appStep}
			if !strings.Contains(appStep.Command, "chef-client") {
				log.Printf("Skipping app step without chef-client; app_step: %s; command: %s", appStep.Name, appStep.Command)
				appSteps = append(appSteps, appStep)
				appStepInfos = append(appStepInfos, appStepInfo)
				continue
			}

			runList, env := parseRunListAndEnv(appStep.Command)
//			log.Printf("RunList: %s, Env: %s", runList, env)
			var output []byte
			if env != "" {
				output = knife("solve", runList, "-E", env)
			} else {
				output = knife("solve", runList)
			}

//			log.Println(string(output))
			cookbooks := parseCookbooks(output)
			appStepInfo.KnifeCookbooks = *cookbooks
//			jsonPrettyPrint(os.Stdout, appStepInfo)

			output = []byte{}
			if env != "" {
				output = knife("environment", "show", env, "-fj")
	//			log.Println(string(output))
				origEnvironment := parseEnvironment(output)
				appStepInfo.OrigEnvironment = *origEnvironment
				appStepInfo.LockedEnvironment = *origEnvironment
				appStepInfo.LockedEnvironment.Name = "l-" + appliance.Name + "-" + strings.Replace(appliance.Version, ".", "-", -1) + "-" + strings.Replace(appStep.Name, " ", "-", -1)

				var lockedCookbookVersions = make(map[string]string)
				for _, cookbook := range appStepInfo.KnifeCookbooks.UCloud {
					lockedCookbookVersions[cookbook.Name] = "= " + cookbook.Version
				}
				for _, cookbook := range appStepInfo.KnifeCookbooks.ThirdParty {
					lockedCookbookVersions[cookbook.Name] = "= " + cookbook.Version
				}
				appStepInfo.LockedEnvironment.CookbookVersions = lockedCookbookVersions

				appStepInfo.LockedAppStep.Name = "l-" + appStep.Name
				appStepInfo.LockedAppStep.Command = strings.Replace(appStep.Command, "-E " + env, "-E " + appStepInfo.LockedEnvironment.Name, 1)

			} else {
				appStepInfo.LockedEnvironment.Name = "l-" + appliance.Name + "-" + strings.Replace(appliance.Version, ".", "-", -1) + "-" + strings.Replace(appStep.Name, " ", "-", -1)
				appStepInfo.LockedEnvironment.JSONClass = "Chef::Environment"
				appStepInfo.LockedEnvironment.ChefType = "environment"
				appStepInfo.LockedEnvironment.DefaultAttributes = make(map[string]interface{})
				appStepInfo.LockedEnvironment.OverridesAttributes = make(map[string]interface{})

				var lockedCookbookVersions = make(map[string]string)
				for _, cookbook := range appStepInfo.KnifeCookbooks.UCloud {
					lockedCookbookVersions[cookbook.Name] = "= " + cookbook.Version
				}
				for _, cookbook := range appStepInfo.KnifeCookbooks.ThirdParty {
					lockedCookbookVersions[cookbook.Name] = "= " + cookbook.Version
				}
				appStepInfo.LockedEnvironment.CookbookVersions = lockedCookbookVersions

				appStepInfo.LockedAppStep.Name = "l-" + appStep.Name
				appStepInfo.LockedAppStep.Command = appStep.Command + " -E " + appStepInfo.LockedEnvironment.Name
			}

			data, err := json.MarshalIndent(appStepInfo.LockedEnvironment, " ", "  ")
			if err != nil {
				log.Fatalf("JSON Marshall: %v", err)
			}

			lockedEnvironmentFile := chefEnvironments + "/" + appStepInfo.LockedEnvironment.Name + ".json"
			if err := ioutil.WriteFile(lockedEnvironmentFile, data, 0755); err != nil {
				log.Fatalf("Failed to write to file: %v", err)
			}

			log.Printf("Creating new chef environment: %s", appStepInfo.LockedEnvironment.Name)
//			output = knife("environment", "from", "file", lockedEnvironmentFile)
//			log.Println(string(output))
//			os.Exit(-1)

			appSteps = append(appSteps, appStepInfo.LockedAppStep)
			appStepInfos = append(appStepInfos, appStepInfo)
			createLockedAppliance = true
		}
		applianceInfo.LockedAppliance.AppSteps = appSteps
		applianceInfo.AppStepInfos = appStepInfos


		if createLockedAppliance {
//			applianceInfo.LockedAppliance.Name = "l-" + appliance.Name
			applianceInfo.LockedAppliance.Version = "1.0.0"

			log.Printf("Creating new appliance %s", applianceInfo.LockedAppliance.Name + "-" + applianceInfo.LockedAppliance.Version)
			jsonPrettyPrint(os.Stdout, applianceInfo)
//			postAppliance(&applianceInfo.LockedAppliance)
//			os.Exit(-1)
		}
	}
}
func postPackage(pkg *Package) {
	packagesURI := ampEndpoint + "/packages"
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(pkg); err != nil {
		log.Fatalf("JSON encode: %v", err)
	}

	req, err := http.NewRequest("POST", packagesURI, &buf)
	if err != nil {
		log.Fatalf("JSON encode: %v", err)
	}

	resp, err := http.DefaultClient.Do(req);
	if err != nil {
		log.Fatalf("POST %s : %v", packagesURI, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		log.Fatalf("Package POST status: %s", resp.Status)
	}

	time.Sleep(100 * time.Millisecond)
}

func postAppliance(appliance *Appliance) {
	appliancesURI := ampEndpoint + "/appliances"
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(appliance); err != nil {
		log.Fatalf("JSON encode: %v", err)
	}

	req, err := http.NewRequest("POST", appliancesURI, &buf)
	if err != nil {
		log.Fatalf("JSON encode: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("POST %s : %v", appliancesURI, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		log.Fatalf("Appliance POST status: %s", resp.Status)
	}

	time.Sleep(100 * time.Millisecond)
}

func getApplianceList() *ApplianceList {
	log.Print("Querying AMP for appliance list ...")
	appliancesURI := ampEndpoint + "/appliances"
	resp, err := http.Get(appliancesURI)
	if err != nil {
		log.Fatalf("GET %s : %v", appliancesURI, err)
	}
	defer resp.Body.Close()

	var applianceList ApplianceList
	if err := json.NewDecoder(resp.Body).Decode(&applianceList); err != nil {
		log.Fatalf("JSON decode: %v", err)
	}

	return &applianceList
}

func getPackageList() *PackageList {
	log.Print("Querying AMP for package list ...")
	packagesURI := ampEndpoint + "/packages"
	resp, err := http.Get(packagesURI)
	if err != nil {
		log.Fatalf("GET %s : %v", packagesURI, err)
	}
	defer resp.Body.Close()

	var packageList PackageList
	if err := json.NewDecoder(resp.Body).Decode(&packageList); err != nil {
		log.Fatalf("JSON decode: %v", err)
	}

	return &packageList
}

func getPackage(packageName string) *Package {
	log.Printf("Querying AMP for package '%s' ...", packageName)
	packageURI := ampEndpoint + "/packages/" + packageName
	resp, err := http.Get(packageURI)
	if err != nil {
		log.Fatalf("GET %s : %v", packageURI, err)
	}
	defer resp.Body.Close()

	var pkg Package
	if err := json.NewDecoder(resp.Body).Decode(&pkg); err != nil {
		log.Fatalf("JSON decode: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	return &pkg
}

func parseCookbooks(data []byte) *Cookbooks {
	cookbooks := Cookbooks{make([]Cookbook, 0), make([]Cookbook, 0)}
	lines := strings.Split(string(data), "\n")
	for _, line := range lines[1 : len(lines)-1] {
		cols := strings.Split(line, " ")
		cookbook := Cookbook{Name: cols[0], Version: cols[1]}
		var ucloudOwned bool
		for _, prefix := range ucloudPrefixes {
			if cookbook.Name == prefix {
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

func parseEnvironment(data []byte) *Environment {
	var origEnvironment Environment
	if err := json.Unmarshal(data, &origEnvironment); err != nil {
		log.Fatalf("JSON decode: %v", err)
	}

	return &origEnvironment
}

func knife(cmd string, args ...string) []byte {
	args = append([]string{cmd}, args...)
	output, err := exec.Command("knife", args...).Output()
	if err != nil {
		log.Fatalf("knife: %v", err)
	}

	return output
}

func getAppliance(applianceURI string) *Appliance {
	info := strings.Split(applianceURI, "/")
	log.Printf("Querying AMP for appliance '%s' ...", info[len(info)-1])
	resp, err := http.Get(applianceURI)
	if err != nil {
		log.Fatalf("GET %s : %v", applianceURI, err)
	}
	defer resp.Body.Close()

	var appliance Appliance
	if err := json.NewDecoder(resp.Body).Decode(&appliance); err != nil {
		log.Fatalf("JSON decode: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	return &appliance
}

func parseRunListAndEnv(command string) (string, string) {
	matches := commandRegex.FindStringSubmatch(command)
//	log.Printf("matches: %v, %d", matches, len(matches))
	if len(matches) != 6 {
		log.Fatalf("parseRoleAndEnv could not parse role and environment from: %s", command)
	}

	return matches[1], matches[4]
}

func jsonPrettyPrint(w io.Writer, v interface{}) {
	data, err := json.MarshalIndent(v, " ", "  ")
	if err != nil {
		log.Fatalf("jsonPrettyPrint: %v", err)
	}
	fmt.Fprintf(w, "%v\n\n", string(data))
}
