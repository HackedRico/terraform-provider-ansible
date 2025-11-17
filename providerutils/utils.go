package providerutils

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
)

/*
	CREATE OPTIONS
*/

const DefaultHostGroup = "default"

type InventoryHost struct {
	Name      string
	Groups    []string
	Variables map[string]string
}

type InventoryGroup struct {
	Name      string
	Children  []string
	Variables map[string]string
}

func InterfaceToString(arr []interface{}) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics

	result := []string{}

	for _, val := range arr {
		tmpVal, ok := val.(string)
		if !ok {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "Error: couldn't parse value to string!",
			})
		}

		result = append(result, tmpVal)
	}

	return result, diags
}

// Create a "verbpse" switch
// example: verbosity = 2 --> verbose_switch = "-vv"
func CreateVerboseSwitch(verbosity int) string {
	verbose := ""

	if verbosity == 0 {
		return verbose
	}

	verbose += "-"
	verbose += strings.Repeat("v", verbosity)

	return verbose
}

func ExpandInventoryHosts(raw []interface{}) ([]InventoryHost, diag.Diagnostics) {
	var diags diag.Diagnostics
	hosts := []InventoryHost{}

	for _, entry := range raw {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "Invalid host definition: expected map input",
			})
			continue
		}

		name, nameOK := entryMap["name"].(string)
		if !nameOK || name == "" {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "Invalid host definition: missing 'name'",
			})
			continue
		}

		groupsRaw, groupsSet := entryMap["groups"].([]interface{})
		if !groupsSet {
			groupsRaw = []interface{}{}
		}
		groups, groupDiags := InterfaceToString(groupsRaw)
		diags = append(diags, groupDiags...)

		variables := map[string]string{}
		if varsRaw, ok := entryMap["variables"].(map[string]interface{}); ok {
			varDiags := mapInterfaceToStringMap(varsRaw, variables)
			diags = append(diags, varDiags...)
		}

		hosts = append(hosts, InventoryHost{
			Name:      name,
			Groups:    groups,
			Variables: variables,
		})
	}

	return hosts, diags
}

func ExpandInventoryGroups(raw []interface{}) ([]InventoryGroup, diag.Diagnostics) {
	var diags diag.Diagnostics
	groups := []InventoryGroup{}

	for _, entry := range raw {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "Invalid group definition: expected map input",
			})
			continue
		}

		name, nameOK := entryMap["name"].(string)
		if !nameOK || name == "" {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "Invalid group definition: missing 'name'",
			})
			continue
		}

		childrenRaw, childrenSet := entryMap["children"].([]interface{})
		if !childrenSet {
			childrenRaw = []interface{}{}
		}
		children, childrenDiags := InterfaceToString(childrenRaw)
		diags = append(diags, childrenDiags...)

		variables := map[string]string{}
		if varsRaw, ok := entryMap["variables"].(map[string]interface{}); ok {
			varDiags := mapInterfaceToStringMap(varsRaw, variables)
			diags = append(diags, varDiags...)
		}

		groups = append(groups, InventoryGroup{
			Name:      name,
			Children:  children,
			Variables: variables,
		})
	}

	return groups, diags
}

func BuildPlaybookInventory(
	inventoryDest string,
	hostname string,
	port int,
	hostgroups []interface{},
	inventoryHosts []InventoryHost,
	inventoryGroups []InventoryGroup,
) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	fileInfo, err := os.CreateTemp("", inventoryDest)
	if err != nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  fmt.Sprintf("Fail to create inventory file: %v", err),
		})
	}

	tempFileName := fileInfo.Name()
	log.Printf("Inventory %s was created", fileInfo.Name())
	defer fileInfo.Close()

	hosts := inventoryHosts
	if len(hosts) == 0 {
		hostGroups, hostGroupDiags := InterfaceToString(hostgroups)
		diags = append(diags, hostGroupDiags...)

		if len(hostGroups) == 0 {
			hostGroups = append(hostGroups, DefaultHostGroup)
		}

		hostVars := map[string]string{}
		if port != -1 {
			hostVars["ansible_port"] = strconv.Itoa(port)
		}

		hosts = append(hosts, InventoryHost{
			Name:      hostname,
			Groups:    hostGroups,
			Variables: hostVars,
		})
	}

	content, buildDiags := buildInventoryFileContent(hosts, inventoryGroups)
	diags = append(diags, buildDiags...)

	if _, err := fileInfo.WriteString(content); err != nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  fmt.Sprintf("Fail to write inventory: %v", err),
		})
	}

	return tempFileName, diags
}

func RemoveFile(filename string) diag.Diagnostics {
	var diags diag.Diagnostics

	err := os.Remove(filename)
	if err != nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  fmt.Sprintf("Fail to remove file %s: %v", filename, err),
		})
	}

	return diags
}

func GetAllInventories(inventoryPrefix string) ([]string, diag.Diagnostics) {
	var diags diag.Diagnostics

	tempDir := os.TempDir()

	log.Printf("[TEMP DIR]: %s", tempDir)

	files, err := os.ReadDir(tempDir)
	if err != nil {
		diags = append(diags, diag.Diagnostic{
			Severity: diag.Error,
			Summary:  fmt.Sprintf("Fail to read dir %s: %v", tempDir, err),
		})
	}

	inventories := []string{}

	for _, file := range files {
		if strings.HasPrefix(file.Name(), inventoryPrefix) {
			inventoryAbsPath := filepath.Join(tempDir, file.Name())
			inventories = append(inventories, inventoryAbsPath)
		}
	}

	return inventories, diags
}

func mapInterfaceToStringMap(input map[string]interface{}, target map[string]string) diag.Diagnostics {
	var diags diag.Diagnostics

	for key, value := range input {
		valueStr, ok := value.(string)
		if !ok {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  fmt.Sprintf("Couldn't parse variable %s to string", key),
			})
			continue
		}

		target[key] = valueStr
	}

	return diags
}

func buildInventoryFileContent(hosts []InventoryHost, groups []InventoryGroup) (string, diag.Diagnostics) {
	var diags diag.Diagnostics

	groupHosts := map[string][]string{}
	groupVars := map[string]map[string]string{}
	groupChildren := map[string][]string{}
	groupNames := map[string]struct{}{}

	for _, group := range groups {
		if group.Name == "" {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "Inventory group is missing a name",
			})
			continue
		}

		groupNames[group.Name] = struct{}{}

		if len(group.Children) > 0 {
			groupChildren[group.Name] = append([]string{}, group.Children...)
		}

		if len(group.Variables) > 0 {
			groupVars[group.Name] = group.Variables
		}
	}

	for _, host := range hosts {
		if host.Name == "" {
			diags = append(diags, diag.Diagnostic{
				Severity: diag.Error,
				Summary:  "Inventory host is missing a name",
			})
			continue
		}

		hostGroups := host.Groups
		if len(hostGroups) == 0 {
			hostGroups = []string{DefaultHostGroup}
		}

		line := formatInventoryHostLine(host.Name, host.Variables)
		for _, groupName := range hostGroups {
			if groupName == "" {
				continue
			}

			groupNames[groupName] = struct{}{}
			groupHosts[groupName] = append(groupHosts[groupName], line)
		}
	}

	var sortedGroupNames []string
	for name := range groupNames {
		sortedGroupNames = append(sortedGroupNames, name)
	}
	sort.Strings(sortedGroupNames)

	var builder strings.Builder
	for _, groupName := range sortedGroupNames {
		builder.WriteString("[" + groupName + "]\n")

		hostLines := groupHosts[groupName]
		sort.Strings(hostLines)
		for _, line := range hostLines {
			builder.WriteString(line)
			builder.WriteString("\n")
		}

		builder.WriteString("\n")

		if vars, ok := groupVars[groupName]; ok && len(vars) > 0 {
			builder.WriteString("[" + groupName + ":vars]\n")
			builder.WriteString(formatInventoryVariables(vars))
			builder.WriteString("\n")
		}

		if children, ok := groupChildren[groupName]; ok && len(children) > 0 {
			builder.WriteString("[" + groupName + ":children]\n")
			sortedChildren := append([]string{}, children...)
			sort.Strings(sortedChildren)
			for _, child := range sortedChildren {
				builder.WriteString(child)
				builder.WriteString("\n")
			}
			builder.WriteString("\n")
		}
	}

	return builder.String(), diags
}

func formatInventoryHostLine(hostname string, variables map[string]string) string {
	line := hostname

	if len(variables) == 0 {
		return line
	}

	keys := make([]string, 0, len(variables))
	for key := range variables {
		keys = append(keys, key)
	}

	sort.Strings(keys)

	for _, key := range keys {
		line += fmt.Sprintf(" %s=%s", key, strconv.Quote(variables[key]))
	}

	return line
}

func formatInventoryVariables(vars map[string]string) string {
	keys := make([]string, 0, len(vars))
	for key := range vars {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for _, key := range keys {
		builder.WriteString(fmt.Sprintf("%s=%s\n", key, strconv.Quote(vars[key])))
	}

	return builder.String()
}
