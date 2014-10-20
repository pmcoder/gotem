package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	//"github.com/dchest/uniuri"
)

//TO create a new file from deps already in place
//1 Get deps from go -list -e -json
//2 get non locals
//3 walk non locals to find repo type and copy
//3 check gotem dir for non locals
//4 pull into gotem
//5 link via existing gopath so that sym link schemes work
//NOTE linking might only work *nix

//TODO might be better to find dirs with repo info and find remotes
var remotes []string = []string{"github"}

type GoList struct {
	Dir     string
	Imports []string
	Deps    []string
}

type RemoteDep struct {
	ImportPath string
	RepoType   string
}

type DepInfo struct {
	DcvsType string
	Version  string
	Path     string
}

func getGoList() *GoList {
	cmd := exec.Command("go", "list", "-e", "-json")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return nil
	}

	gList, err := buildGoList(out.Bytes())
	if err != nil {

	}

	return gList

}

func findRemoteDeps(deps []string) []string {
	remoteDeps := []string{}
	for _, dep := range deps {
		fmt.Println("Checking %s for remote", dep)
		for _, remoteSign := range remotes {
			if strings.Contains(dep, remoteSign) {
				remoteDeps = append(remoteDeps, dep)
				continue
			}
		}
	}

	return remoteDeps
}

func buildGoList(j []byte) (*GoList, error) {

	gList := &GoList{}

	err := json.Unmarshal(j, gList)
	if err != nil {
		return nil, err
	}

	return gList, nil
}

func getGoPath() string {

	for _, env := range os.Environ() {
		if strings.Contains(env, "GOPATH") {
			g := strings.Split(env, "=")
			if len(g) > 1 {
				return g[1]
			}
		}
	}

	return ""

}

//return dep info
func walkGoPathForRemotes(r []string, goPath string) {

	depInfos := []DepInfo{}
	goPaths := strings.Split(goPath, ":")

	for _, remote := range r {
		remotePath := ""
		for _, gp := range goPaths {
			remotePath = filepath.Join(gp, "src", remote)
			fmt.Printf("Checking path %s\n", remotePath)
			//make sure the dir is there
			_, err := os.Stat(remotePath)
			if err != nil {
				fmt.Println(err)
				if os.IsNotExist(err) {
					//mark as not go gotten... maybe even go get it
					fmt.Printf("%s does not exist\n", remotePath)
				}
				//TODO do better existence check handling
			}
			break //the actual repo was found so don't keep searching gopath
		}

		if remotePath == "" {
			fmt.Println("Can't find dep locally")
			continue
		}

		depInfo := processDep(remote, remotePath)
		depInfos = append(depInfos, *depInfo)
		//copy the dir
		err := copyDep(remote, remotePath)
		if err != nil {
			fmt.Println("Error copying files", err)
		}
		fmt.Println(remotePath)
	}

	saveDeps(depInfos)
}

func pathExists(path string) bool {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}

	return true
}

func copyDep(remotePath, localPath string) error {

	localPathInfo, err := os.Stat(localPath)
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Join("_gotem", remotePath), localPathInfo.Mode())
	if err != nil {
		fmt.Println("Could not make dirs under _gotem")
		return err
	}

	srcDir, _ := os.Open(localPath)
	object, err := srcDir.Readdir(-1)

	for _, obj := range object {
		srcFile := filepath.Join(localPath, obj.Name())
		destFile := filepath.Join("_gotem", remotePath, obj.Name())

		if obj.IsDir() {
			err = copyDep(remotePath, srcFile)
			if err != nil {
				return err
			}
		} else {
			sf, err := os.Open(srcFile)
			if err != nil {
				return err
			}
			defer sf.Close()
			df, err := os.Create(destFile)
			if err != nil {
				return err
			}
			defer df.Close()

			_, err = io.Copy(df, sf)
			if err != nil {
				return err
			}
			srcInfo, err := os.Stat(srcFile)
			if err != nil {
				err = os.Chmod(destFile, srcInfo.Mode())
			}
		}

	}

	return nil
}

func saveDeps(deps []DepInfo) error {
	//check for existence of the gotem file
	/*if !pathExists("gotem.json"){
		//create the json file
	}*/

	gotem, err := os.Create("gotem.json")
	if err != nil {
		fmt.Println("Can't open or create gotem.json", err)
	}

	//write the dep info to file
	b, err := json.Marshal(deps)
	if err != nil {
		fmt.Println("Error marshaling gotem in")
		return err
	}
	_, err = gotem.Write(b)
	if err != nil {
		fmt.Println("Error writing gotem file")
		return err
	}

	return nil

}

func getDepRepo(path string) string {

	if pathExists(filepath.Join(path, ".git")) {
		return "git"
	} else if pathExists(filepath.Join(path, ".svn")) {
		return "svn"
	} else {
		return ""
	}

}

func addDepsToPath() {

}

func linkDepstToPath() {

}

func findDependencyVersion(depInfo *DepInfo) error {
	//only handle git for now
	if depInfo.DcvsType != "git" {
		return errors.New("Unsupported repo type " + depInfo.DcvsType)
	}

	tagOut, err := exec.Command("git", "describe", "--tags").Output()
	if err != nil {
		fmt.Println("Error looking for tag... moving to revision num", err)
		err = nil
	} else {
		if len(tagOut) > 0 && string(tagOut[0:5]) != "fatal" {
			depInfo.Version = string(tagOut[:len(tagOut)-1])
			return nil
		}
	}

	idOut, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		fmt.Println("Error finding revision num.... giving up", err)
		return err
	}

	if len(idOut) > 0 {
		depInfo.Version = string(idOut[:len(idOut)-1])
	} else {
		fmt.Println("Could not get tag or version")
		return errors.New("Could not get tag or version")
	}
	return nil
}

func processDep(remotePath, localPath string) *DepInfo {
	depInfo := DepInfo{DcvsType: getDepRepo(localPath), Path: remotePath}

	findDependencyVersion(&depInfo)

	return &depInfo
}

func main() {
	//uniuri.New()
	gl := getGoList()
	fmt.Println("Deps:", gl.Deps)
	r := findRemoteDeps(gl.Deps)
	fmt.Printf("Remotes %v\n", r)
	goPath := getGoPath()
	fmt.Println(getGoPath())
	walkGoPathForRemotes(r, goPath)
}
