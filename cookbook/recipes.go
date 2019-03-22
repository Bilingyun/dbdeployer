// DBDeployer - The MySQL Sandbox
// Copyright © 2006-2019 Giuseppe Maxia
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package cookbook

import (
	"fmt"
	"github.com/alexeyco/simpletable"
	"github.com/datacharmer/dbdeployer/common"
	"github.com/datacharmer/dbdeployer/defaults"
	"github.com/datacharmer/dbdeployer/globals"
	"os"
	"path"
	"regexp"
	"strings"
)

const (
	ErrNoVersionFound = 1
	ErrNoRecipeFound  = 2
	VersionNotFound   = "NOTFOUND"
)

var (
	AuxiliaryRecipes        = []string{"prerequisites", "include"}
	PrerequisitesShown bool = false
)

func ListRecipes() {
	table := simpletable.New()

	table.Header = &simpletable.Header{
		Cells: []*simpletable.Cell{
			{Align: simpletable.AlignCenter, Text: "recipe"},
			{Align: simpletable.AlignCenter, Text: "script name"},
			{Align: simpletable.AlignCenter, Text: "description"},
			{Align: simpletable.AlignCenter, Text: "needed\n flavor"},
		},
	}

	for name, template := range RecipesList {
		if template.IsExecutable {
			var cells []*simpletable.Cell
			cells = append(cells, &simpletable.Cell{Text: name})
			cells = append(cells, &simpletable.Cell{Text: template.ScriptName})
			cells = append(cells, &simpletable.Cell{Text: template.Description})
			cells = append(cells, &simpletable.Cell{Text: template.RequiredFlavor})
			table.Body.Cells = append(table.Body.Cells, cells)
		}
	}
	table.SetStyle(simpletable.StyleRounded)
	table.Println()
}

func getCookbookDirectory() string {
	cookbookDir := defaults.Defaults().CookbookDirectory
	if !common.DirExists(cookbookDir) {
		err := os.Mkdir(cookbookDir, globals.PublicDirectoryAttr)
		if err != nil {
			common.Exitf(1, "error creating cookbook directory %s: %s", cookbookDir, err)
		}
	}
	return cookbookDir
}

func recipeExists(recipeName string) bool {
	_, ok := RecipesList[recipeName]
	if ok {
		return true
	}
	return false
}

func createPrerequisites() string {
	cookbookDir := getCookbookDirectory()
	preReqScript := path.Join(cookbookDir, CookbookPrerequisites)
	for _, recipeName := range AuxiliaryRecipes {
		CreateRecipe(recipeName, "")
	}
	return preReqScript
}

func showPrerequisites(flavor string) {
	if PrerequisitesShown {
		return
	}
	prerequisitesScript := createPrerequisites()
	fmt.Printf("No tarballs for flavor %s were found in your environment\n", flavor)
	fmt.Printf("Please read instructions in %s\n", prerequisitesScript)
	PrerequisitesShown = true
}

func ShowRecipe(recipeName string, flavor string, raw bool) {
	if !recipeExists(recipeName) {
		fmt.Printf("recipe %s not found\n", recipeName)
		os.Exit(1)
	}
	if raw {
		fmt.Printf("%s\n", RecipesList[recipeName].Contents)
		return
	}
	recipe := RecipesList[recipeName]
	if recipe.RequiredFlavor != "" && flavor == "" {
		flavor = recipe.RequiredFlavor
	}
	if flavor == "" {
		flavor = common.MySQLFlavor
	}
	recipeText, err, _ := GetRecipe(recipeName, flavor)
	if err != nil {
		showPrerequisites(flavor)
	}
	fmt.Printf("%s\n", recipeText)
}

func CreateRecipe(recipeName, flavor string) {
	var isRecursive bool = false

	for _, auxRecipeName := range AuxiliaryRecipes {
		if auxRecipeName == recipeName {
			isRecursive = true
		}
	}
	if strings.ToLower(recipeName) == "all" {
		for name, _ := range RecipesList {
			CreateRecipe(name, flavor)
		}
		return
	}
	recipe := RecipesList[recipeName]
	if recipe.RequiredFlavor != "" {
		flavor = recipe.RequiredFlavor
	}
	if flavor == "" {
		flavor = common.MySQLFlavor
	}
	if !recipeExists(recipeName) {
		fmt.Printf("recipe %s not found\n", recipeName)
		os.Exit(1)
	}
	recipeText, err, versionCode := GetRecipe(recipeName, flavor)
	if err != nil && !isRecursive {
		showPrerequisites(flavor)
		common.Exitf(1, "error getting recipe %s: %s", recipeName, err)
	}
	if versionCode == ErrNoVersionFound && !isRecursive {
		showPrerequisites(flavor)
	}
	cookbookDir := getCookbookDirectory()
	if recipe.ScriptName != CookbookInclude {
		targetInclude := path.Join(cookbookDir, CookbookInclude)
		if !common.FileExists(targetInclude) && !isRecursive {
			CreateRecipe("include", flavor)
		}
	}
	targetScript := path.Join(cookbookDir, recipe.ScriptName)
	//if common.FileExists(targetScript) {
	//	fmt.Printf("Script %s already created\n", targetScript)
	//	return
	//}
	err = common.WriteString(recipeText, targetScript)
	if err != nil {
		common.Exitf(1, "error writing file %s: %s", targetScript, err)
	}
	if recipe.IsExecutable {
		err = os.Chmod(targetScript, globals.ExecutableFileAttr)
		if err != nil {
			common.Exitf(1, "error while making file %s executable: %s", targetScript, err)
		}
	}
	fmt.Printf("%s created\n", targetScript)
}

func GetLatestVersion(wantedVersion, flavor string) string {
	if wantedVersion == "" {
		wantedVersion = os.Getenv("WANTED_VERSION")
	}
	sandboxBinary := os.Getenv("SANDBOX_BINARY")
	if sandboxBinary == "" {
		sandboxBinary = defaults.Defaults().SandboxBinary
	}
	versions := common.GetFlavoredVersionsFromDir(sandboxBinary, flavor)
	if len(versions) == 0 {
		return VersionNotFound + "_" + flavor
	}

	sortedVersions := common.SortVersionsSubset(versions, wantedVersion)
	if len(sortedVersions) < 1 {
		return VersionNotFound + "_" + flavor
	}
	latestVersion := sortedVersions[len(sortedVersions)-1]
	return latestVersion
}

func GetRecipe(recipeName, flavor string) (string, error, int) {
	var text string

	recipe, ok := RecipesList[recipeName]
	if !ok {
		return text, fmt.Errorf("recipe %s not found", recipeName), ErrNoRecipeFound
	}
	latestVersions := make(map[string]string)
	for _, version := range []string{"5.0", "5.1", "5.5", "5.6", "5.7", "8.0"} {
		latest := GetLatestVersion(version, common.MySQLFlavor)
		if latest != "" {
			latestVersions[version] = latest
		} else {
			latestVersions[version] = fmt.Sprintf("%s_%s", VersionNotFound, version)
		}
	}
	latestVersion := GetLatestVersion("", flavor)
	versionCode := 0
	if latestVersion == VersionNotFound {
		versionCode = ErrNoVersionFound
	}
	var data = common.StringMap{
		"Copyright":     globals.Copyright,
		"TemplateName":  recipeName,
		"LatestVersion": latestVersion,
	}
	for version, latest := range latestVersions {
		reDot := regexp.MustCompile(`\.`)
		versionName := reDot.ReplaceAllString(version, "_")
		fieldName := fmt.Sprintf("Latest%s", versionName)
		data[fieldName] = latest
	}
	text, err := common.SafeTemplateFill(recipeName, recipe.Contents, data)
	if err != nil {
		return globals.EmptyString, err, versionCode
	}
	return text, nil, versionCode
}
