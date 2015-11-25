package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	// thirdparty lib
	"github.com/go-ini/ini"
	"github.com/smallfish/simpleyaml"
	//"gopkg.in/yaml.v2"
)

const (
	VERSION = "0.1.0"
)

const (
	ANSIBLE_CMD = "/usr/bin/ansible-playbook"
)

const (
	retOK = iota
	retFailed
	retInvaidArgs
)

func execCmd(cmd string, shell bool) (out []byte, err error) {
	fmt.Printf("run command: %s", cmd)
	if shell {
		out, err = exec.Command("bash", "-c", cmd).Output()
	} else {
		out, err = exec.Command(cmd).Output()
	}
	return out, err
}

func checkExistFiles(files ...string) bool {
	loginfo := "checkExistFiles"

	for _, file := range files {
		file = strings.TrimSpace(file)
		if _, err := isExists(file); err != nil {
			fmt.Printf("[ERROR] %s() - %s.\n", loginfo, err)
			return false
		}
	}
	return true
}

func isExists(file string) (ret bool, err error) {
	// equivalent to Python's `if not os.path.exists(filename)`
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return false, err
	} else {
		return true, nil
	}
}

func displayYamlFile(file string) {
	loginfo := "displayYamlFile"

	if _, err := isExists(file); err != nil {
		fmt.Printf("[ERROR] %s() - %s.\n", loginfo, err)
		panic(err)
	}

	contents, err := ioutil.ReadFile(file)
	if err != nil {
		panic(err)
	}

	println(string(contents))
}

func getSingleHostname(file string, section string) string {
	var ini_cfg *ini.File
	loginfo := "getSingleHostname"

	hostname := ""

	ini_cfg, err := ini.Load(file)
	if err != nil || ini_cfg == nil {
		fmt.Printf("[ERROR] %s() - %s.\n", loginfo, err)
		return hostname
	}

	items, err := ini_cfg.GetSection(section)

	for _, v := range items.KeyStrings() {
		v = strings.TrimSpace(v)

		if v != "" {
			tokens := strings.Split(v, " ")

			if len(tokens) == 0 {
				continue
			}

			host := tokens[0]

			if strings.HasPrefix(host, "(") && strings.HasSuffix(host, ")") {
				srp := strings.NewReplacer("(", "", ")", "")
				host = srp.Replace(host)
			} else if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
				srp := strings.NewReplacer("[", "", "]", "")
				host = srp.Replace(host)
			}

			tokens = strings.Split(host, " ")
			if len(tokens) == 0 {
				continue
			}
			hostname = tokens[0]

			// Three cases to check:
			// 0. A hostname that contains a range pesudo-code and a port
			// 1. A hostname that contains just a port
			if strings.Count(hostname, ":") > 1 {
				// Possible an IPv6 address, or maybe a host line with multiple ranges
				// IPv6 with Port  XXX:XXX::XXX.port
				// FQDN            foo.example.com
				if strings.Count(hostname, ".") == 1 {
					hostname = hostname[0:strings.LastIndex(hostname, ".")]
				}
			} else if (strings.Count(hostname, "[") > 0 && strings.Count(hostname, "]") > 0 &&
				(strings.LastIndex(hostname, "]") < strings.LastIndex(hostname, ":"))) ||
				((strings.Count(hostname, "]") <= 0) && (strings.Count(hostname, ":") > 0)) {
				hostname = hostname[0:strings.LastIndex(hostname, ":")]
			}
		}

		if hostname != "" {
			break
		}
	}

	return hostname
}

func doUpdateAction(action string, inventory_file string, operation_file string,
	version string, concurrent int) {

	loginfo := "doUpdateAction"
	var cmd, ext_vars string

	if action != "update" || inventory_file == "" || operation_file == "" {
		fmt.Printf("Error parameters in %s\n", loginfo)
		os.Exit(retFailed)
	}

	if version != "" {
		version = strings.TrimSpace(version)
		ext_vars = fmt.Sprintf(" version=%s ", version)
	} else {
		ext_vars = ""
	}

	if ext_vars != "" {
		cmd = fmt.Sprintf("%s -i %s %s --extra-vars \"forks=%d hosts=%s %s\" -t %s -f %d ",
			ANSIBLE_CMD, inventory_file, operation_file, concurrent, action, ext_vars, action, concurrent)
	} else {
		cmd = fmt.Sprintf("%s -i %s %s --extra-vars \"forks=%d hosts=%s \" -t %s -f %d ",
			ANSIBLE_CMD, inventory_file, operation_file, concurrent, action, action, concurrent)
	}

	fmt.Printf("%s\n", cmd)
}

func doDeployAction(action string, inventory_file string, operation_file string,
	singlemode bool, concurrent int, retry_file string, ext_vars string, section string) {

	loginfo := "doDeployAction"
	var cmd string

	if action != "deploy" || inventory_file == "" || operation_file == "" {
		fmt.Printf("Error parameters in %s", loginfo)
		os.Exit(retFailed)
	}

	section = strings.TrimSpace(section)
	if section != "" {
		action = section
	}

	if ext_vars != "" {
		cmd = fmt.Sprintf("%s -i %s %s --extra-vars \"forks=%d hosts=%s %s\" -t %s -f %d ",
			ANSIBLE_CMD, inventory_file, operation_file, concurrent, action, ext_vars, action, concurrent)
	} else {
		cmd = fmt.Sprintf("%s -i %s %s --extra-vars \"forks=%d hosts=%s \" -t %s -f %d ",
			ANSIBLE_CMD, inventory_file, operation_file, concurrent, action, action, concurrent)
	}

	if singlemode {
		hostname := getSingleHostname(inventory_file, action)
		fmt.Printf("%s\n", hostname)

		if hostname != "" {
			cmd = fmt.Sprintf("%s -l %s ", cmd, strings.TrimSpace(hostname))
		} else {
			fmt.Printf("Error: %s() get single hostname from %s failed in single mode.", loginfo, retry_file)
			os.Exit(retFailed)
		}
	} else if retry_file != "" {
		fmt.Printf("%s\n", retry_file)
	}

	fmt.Printf("%s\n", cmd)
}

var Usage = func() {
	fmt.Fprintf(os.Stdout, "Usage of %s [options] action\n", os.Args[0])
	flag.PrintDefaults()

	fmt.Fprintf(os.Stdout, "\n  action    action to do required:(check,update,deploy,rollback).\n")
}

func main() {
	var err error
	var ini_cfg *ini.File

	flag.Usage = Usage

	single_mode := flag.Bool("s", false, "Single mode in deploy one host for observation.")
	concurrent := flag.Int("c", 1, "Process nummber for run the command at same time.")
	program_version := flag.String("V", "", "Module program version for deploy.")
	extra_vars := flag.String("e", "", "Extra vars for ansible-playbook.")
	section := flag.String("S", "", "Inventory section for distinguish hosts or tags.")
	retry_file := flag.String("r", "", "Retry file for ansible redo failed hosts.")
	inventory_file := flag.String("i", "", "Specify inventory host file.")
	operation_file := flag.String("f", "", "File name for module configure(yml format).")
	version := flag.Bool("v", false, "show version")

	flag.Parse()

	if *version {
		fmt.Printf("%s: %s\n", os.Args[0], VERSION)
		os.Exit(retOK)
	}

	var action string = flag.Arg(0)

	if *operation_file == "" || *inventory_file == "" {
		fmt.Printf("[ERROR] operation and inventory file must provide.\n\n")
		flag.Usage()
		os.Exit(retFailed)
	} else {
		ret := checkExistFiles(*operation_file, *inventory_file)
		if !ret {
			fmt.Printf("[ERROR] check exists of operation and inventory file.\n")
			os.Exit(retInvaidArgs)
		}
	}

	if action == "" {
		fmt.Printf("[ERROR] action(check,update,deploy,rollback) must provide one.\n\n")
		flag.Usage()
		os.Exit(retFailed)
	}

	if *operation_file, err = filepath.Abs(*operation_file); err != nil {
		panic(err)
	}

	if *inventory_file, err = filepath.Abs(*inventory_file); err != nil {
		panic(err)
	} else {
		ini_cfg, err = ini.Load(*inventory_file)
		if err != nil {
			panic(err)
		}
	}

	if *operation_file, err = filepath.Abs(*operation_file); err != nil {
		panic(err)
	} else {
		var data []byte
		f, err := os.Open(*operation_file)
		if data, err = ioutil.ReadAll(f); err != nil {
			panic(err)
		} else {
			defer f.Close() // f.Close will run when we're finished.
			//yml_cfg, err = i.Load(*inventory_file)
			_, err = simpleyaml.NewYaml(data)
			if err != nil {
				panic(err)
			}
		}
	}

	fmt.Printf("[%s] action on [%s]\n", action, *operation_file)
	switch action {
	case "check":
		fmt.Printf("-------------Now doing in action: %s\n", action)
		fmt.Println("check configure file.")
		ini_cfg.WriteTo(os.Stdout)
		fmt.Println("")
		displayYamlFile(*operation_file)
	case "update":
		fmt.Printf("-------------Now doing in action: %s\n", action)
		fmt.Println("update code.")
		doUpdateAction(action, *inventory_file, *operation_file, *program_version, *concurrent)
	case "deploy":
		fmt.Printf("-------------Now doing in action: %s\n", action)
		fmt.Println("deploy code.")
		doDeployAction(action, *inventory_file, *operation_file, *single_mode, *concurrent, *retry_file, *extra_vars, *section)
	case "rollback":
		fmt.Printf("-------------Now doing in action: %s\n", action)
		fmt.Println("rollback code.")
	default:
		fmt.Println("Not supported action: %s\n", action)
		os.Exit(retFailed)
	}
}
