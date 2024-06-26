/*
 * Copyright (c) 2024-present unTill Pro, Ltd.
 * @author Alisher Nurmanov
 */

package main

import (
	"bytes"
	"embed"
	"fmt"
	"go/format"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"github.com/spf13/cobra"

	"github.com/voedger/voedger/pkg/appdef"
	"github.com/voedger/voedger/pkg/compile"
	"github.com/voedger/voedger/pkg/sys"
	coreutils "github.com/voedger/voedger/pkg/utils"
)

//go:embed ormtemplates/*
var ormTemplatesFS embed.FS
var reservedWords = []string{"type"}

func newOrmCmd(params *vpmParams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "orm",
		Short: "generate orm for package",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			compileRes, err := compile.Compile(params.Dir)
			if err != nil {
				return err
			}
			return generateOrm(compileRes, params)
		},
	}
	cmd.Flags().StringVarP(&params.HeaderFile, "header-file", "", "", "path to file to insert as a header to generated files")
	return cmd
}

// generateOrm generates ORM from the given working directory
func generateOrm(compileRes *compile.Result, params *vpmParams) error {
	dir, err := createOrmDir(params.Dir)
	if err != nil {
		return err
	}

	headerContent, err := getHeaderFileContent(params.HeaderFile)
	if err != nil {
		return err
	}

	iTypeObjs, pkgInfos, currentPkgLocalName := getPkgAppDefObjs(compileRes.ModulePath, compileRes.AppDef, headerContent)
	pkgData := getOrmData(currentPkgLocalName, pkgInfos, iTypeObjs)
	if err := generateOrmFiles(pkgData, dir); err != nil {
		return err
	}
	// update dependencies if go.mod file exists
	if err := execGoModTidy(dir); err != nil {
		return err
	}
	return nil
}

// getPkgAppDefObjs gathers objects from the current package
// and returns a list of objects, a map of package local names to its info and the current package local name
func getPkgAppDefObjs(packagePath string, appDef appdef.IAppDef, headerContent string) (iTypeObjs []appdef.IType, pkgInfos map[string]ormPackageInfo, currentPkgLocalName string) {
	uniqueObjects := make([]string, 0)
	iTypeObjs = make([]appdef.IType, 0)        // list of package objects
	pkgInfos = make(map[string]ormPackageInfo) // mapping of package local names to its info
	// sys package is implicitly added to the list of packages,
	// so we need to add it manually
	currentPkgLocalName = appdef.SysPackage
	pkgInfos[appdef.SysPackage] = ormPackageInfo{
		Name:              appdef.SysPackage,
		FullPath:          sys.PackagePath,
		HeaderFileContent: headerContent,
	}
	appDef.Packages(func(localName, fullPath string) {
		if fullPath == packagePath {
			currentPkgLocalName = localName
		}
		pkgInfos[localName] = ormPackageInfo{
			Name:              localName,
			FullPath:          fullPath,
			HeaderFileContent: headerContent,
		}
	})

	collectITypeObjs := func(iTypeObj appdef.IType) {
		// skip abstract types
		if iAbstract, ok := iTypeObj.(appdef.IWithAbstract); ok {
			if iAbstract.Abstract() {
				return
			}
		}
		qName := iTypeObj.QName()
		// skip types from other packages
		if qName.Pkg() != currentPkgLocalName {
			return
		}
		if !slices.Contains(uniqueObjects, qName.String()) {
			iTypeObjs = append(iTypeObjs, iTypeObj)
			uniqueObjects = append(uniqueObjects, qName.String())
		}
	}

	// gather objects from the current package
	appDef.Types(func(iTypeObj appdef.IType) {
		if workspace, ok := iTypeObj.(appdef.IWorkspace); ok {
			workspace.Types(collectITypeObjs)
		}
	})
	return
}

func generateOrmFiles(pkgData map[ormPackageInfo][]interface{}, dir string) error {
	ormFiles := make([]string, 0, len(pkgData)+1)
	for pkgInfo, pkgItems := range pkgData {
		ormPkgData := ormPackage{
			ormPackageInfo: pkgInfo,
			Items:          pkgItems,
		}
		ormFilePath, err := generateOrmFile(pkgInfo.Name, ormPkgData, dir)
		if err != nil {
			return fmt.Errorf(errInGeneratingOrmFileFormat, ormFilePath, err)
		}
		ormFiles = append(ormFiles, ormFilePath)
	}

	sysFilePath := filepath.Join(dir, "types.go")
	ormFiles = append(ormFiles, sysFilePath)
	if err := os.WriteFile(sysFilePath, []byte(sysContent), coreutils.FileMode_rw_rw_rw_); err != nil {
		return fmt.Errorf(errInGeneratingOrmFileFormat, sysFilePath, err)
	}

	return formatOrmFiles(ormFiles)
}

func formatOrmFiles(ormFiles []string) error {
	for _, ormFile := range ormFiles {
		ormFileContent, err := os.ReadFile(ormFile)
		if err != nil {
			return err
		}

		formattedContent, err := format.Source(ormFileContent)
		if err != nil {
			return err
		}

		if err := os.WriteFile(ormFile, formattedContent, coreutils.FileMode_rw_rw_rw_); err != nil {
			return err
		}
	}
	return nil
}

func generateOrmFile(localName string, ormPkgData ormPackage, dir string) (filePath string, err error) {
	filePath = filepath.Join(dir, fmt.Sprintf("package_%s.go", localName))
	ormFileContent, err := fillInTemplate(ormPkgData)
	if err != nil {
		return filePath, err
	}

	if err := os.WriteFile(filePath, ormFileContent, coreutils.FileMode_rw_rw_rw_); err != nil {
		return filePath, err
	}
	return filePath, nil
}

func getOrmData(localName string, pkgInfos map[string]ormPackageInfo, iTypeObjs []appdef.IType) (pkgData map[ormPackageInfo][]interface{}) {
	pkgData = make(map[ormPackageInfo][]interface{})
	uniquePkgQNames := make(map[ormPackageInfo][]string)
	for _, obj := range iTypeObjs {
		processITypeObj(localName, pkgInfos, pkgData, uniquePkgQNames, obj)
	}
	return
}

func newPackageItem(defaultLocalName string, pkgInfos map[string]ormPackageInfo, obj interface{}) ormPackageItem {
	name := getName(obj)
	qName := obj.(appdef.IType).QName()
	localName := defaultLocalName
	if obj != nil {
		localName = qName.Pkg()
	}
	pkgInfo := pkgInfos[localName]
	return ormPackageItem{
		Package:   pkgInfo,
		QName:     qName.String(),
		TypeQName: fmt.Sprintf("%s.%s", pkgInfo.FullPath, name),
		Name:      name,
		Type:      getObjType(obj),
	}
}

func newFieldItem(tableData ormTableItem, field appdef.IField) ormField {
	name := normalizeName(field.Name())
	return ormField{
		Table:         tableData,
		Type:          getFieldType(field),
		Name:          normalizeName(field.Name()),
		GetMethodName: fmt.Sprintf("Get_%s", strings.ToLower(name)),
		SetMethodName: fmt.Sprintf("Set_%s", strings.ToLower(name)),
	}
}

func processITypeObj(localName string, pkgInfos map[string]ormPackageInfo, pkgData map[ormPackageInfo][]interface{}, uniquePkgQNames map[ormPackageInfo][]string, obj appdef.IType) (newItem interface{}) {
	if obj == nil {
		return nil
	}

	pkgItem := newPackageItem(localName, pkgInfos, obj)
	if pkgItem.Type == unknownType {
		return nil
	}

	switch t := obj.(type) {
	case appdef.ICDoc, appdef.IWDoc, appdef.IView, appdef.IODoc, appdef.IObject:
		tableData := ormTableItem{
			ormPackageItem: pkgItem,
			Fields:         make([]ormField, 0),
		}

		iView, isView := t.(appdef.IView)
		if isView {
			for _, key := range iView.Key().Fields() {
				fieldItem := newFieldItem(tableData, key)
				if fieldItem.Type == unknownType {
					continue
				}
				tableData.Keys = append(tableData.Keys, fieldItem)
			}
		}
		// fetching fields
		for _, field := range t.(appdef.IFields).Fields() {
			fieldItem := newFieldItem(tableData, field)
			if fieldItem.Type == unknownType {
				continue
			}

			isKey := false
			for _, key := range tableData.Keys {
				if key.Name == fieldItem.Name {
					isKey = true
					break
				}
			}
			if !isKey {
				tableData.Fields = append(tableData.Fields, fieldItem)
			}
		}
		newItem = tableData
	case appdef.ICommand, appdef.IQuery:
		var resultFields []ormField
		argumentObj := processITypeObj(localName, pkgInfos, pkgData, uniquePkgQNames, t.(appdef.IFunction).Param())

		var unloggedArgumentObj interface{}
		if iCommand, ok := t.(appdef.ICommand); ok {
			unloggedArgumentObj = processITypeObj(localName, pkgInfos, pkgData, uniquePkgQNames, iCommand.UnloggedParam())
		}
		if resultObj := processITypeObj(localName, pkgInfos, pkgData, uniquePkgQNames, t.(appdef.IFunction).Result()); resultObj != nil {
			if tableData, ok := resultObj.(ormTableItem); ok {
				resultFields = tableData.Fields
			}
		}

		commandItem := ormCommand{
			ormPackageItem:         pkgItem,
			ArgumentObject:         argumentObj,
			ResultObjectFields:     resultFields,
			UnloggedArgumentObject: unloggedArgumentObj,
		}
		newItem = commandItem
	default:
		typeKind := t.Kind()
		if typeKind == appdef.TypeKind_Object {
			return processITypeObj(localName, pkgInfos, pkgData, uniquePkgQNames, t.(appdef.IObject))
		}
		newItem = pkgItem
	}
	// add new package item to the package data
	if !slices.Contains(uniquePkgQNames[pkgItem.Package], getQName(newItem)) {
		pkgData[pkgItem.Package] = append(pkgData[pkgItem.Package], newItem)
		uniquePkgQNames[pkgItem.Package] = append(uniquePkgQNames[pkgItem.Package], getQName(newItem))
	}
	return
}

func fillInTemplate(ormPkgData ormPackage) ([]byte, error) {
	ormTemplates, err := fs.Sub(ormTemplatesFS, "ormtemplates")
	if err != nil {
		return nil, fmt.Errorf("failed to read templates directory: %w", err)
	}
	t, err := template.New("package").Funcs(template.FuncMap{
		"capitalize": func(s string) string {
			if len(s) == 0 {
				return s
			}
			return strings.ToUpper(s[:1]) + s[1:]
		},
		"lower": strings.ToLower,
	}).ParseFS(ormTemplates, "*")
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	var filledTemplate bytes.Buffer
	if err := t.ExecuteTemplate(&filledTemplate, "package", ormPkgData); err != nil {
		return nil, fmt.Errorf("failed to fill template: %w", err)
	}

	return filledTemplate.Bytes(), nil
}

func getHeaderFileContent(headerFilePath string) (string, error) {
	if headerFilePath == "" {
		return defaultOrmFilesHeaderComment, nil
	}

	headerFileContent, err := os.ReadFile(headerFilePath)
	if err != nil {
		return "", err
	}

	return string(headerFileContent), nil
}

func createOrmDir(dir string) (string, error) {
	ormDirPath := filepath.Join(dir, wasmDirName, ormDirName)
	exists, err := coreutils.Exists(ormDirPath)
	if err != nil {
		// notest
		return "", err
	}
	if exists {
		if err := os.RemoveAll(ormDirPath); err != nil {
			return "", err
		}
	}
	return ormDirPath, os.MkdirAll(ormDirPath, coreutils.FileMode_rwxrwxrwx)
}

func normalizeName(name string) (newName string) {
	newName = strings.ReplaceAll(name, ".", "_")
	if slices.Contains(reservedWords, strings.ToLower(newName)) {
		newName += "_"
	}
	return
}

func getName(obj interface{}) string {
	if obj == nil {
		return ""
	}
	return normalizeName(obj.(appdef.IType).QName().Entity())
}

func getObjType(obj interface{}) string {
	switch t := obj.(type) {
	case appdef.IODoc:
		return "ODoc"
	case appdef.ICDoc:
		if t.Singleton() {
			return "CSingleton"
		}
		return "CDoc"
	case appdef.IWDoc:
		if t.Singleton() {
			return "WSingleton"
		}
		return "WDoc"
	case appdef.IView:
		return "View"
	case appdef.ICommand:
		return "Command"
	case appdef.IQuery:
		return "Query"
	case appdef.IObject:
		return getTypeKind(t.Kind())
	case appdef.IType:
		return getTypeKind(t.Kind())
	default:
		return unknownType
	}
}

func getTypeKind(typeKind appdef.TypeKind) string {
	switch typeKind {
	case appdef.TypeKind_Object:
		return "Type"
	case appdef.TypeKind_CDoc:
		return "CDoc"
	case appdef.TypeKind_WDoc:
		return "WDoc"
	case appdef.TypeKind_ODoc:
		return "ODoc"
	default:
		return unknownType
	}
}

func getFieldType(field appdef.IField) string {
	switch field.DataKind() {
	case appdef.DataKind_bool:
		return "bool"
	case appdef.DataKind_int32:
		return "int32"
	case appdef.DataKind_int64:
		return "int64"
	case appdef.DataKind_float32:
		return "float32"
	case appdef.DataKind_float64:
		return "float64"
	case appdef.DataKind_bytes:
		return "Bytes"
	case appdef.DataKind_string:
		return "string"
	case appdef.DataKind_RecordID:
		return "Ref"
	default:
		return unknownType
	}
}
