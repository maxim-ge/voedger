/*
 * Copyright (c) 2021-present Sigma-Soft, Ltd.
 */

package istructsmem

import (
	"context"
	"encoding/json"
	"log"

	"github.com/stretchr/testify/require"
	"github.com/untillpro/goutils/logger"
	"github.com/voedger/voedger/pkg/iratesce"
	"github.com/voedger/voedger/pkg/istructs"
	"github.com/voedger/voedger/pkg/schemas"

	"testing"
)

/* Пояснения к тесту. */
// Некто, представившийся как «Карлосон 哇"呀呀» совершал покупку в супермаркете № 1234.
// Пока он выкладывал покупки на кассе самообслуживания № 762, автоматические средства магазина сфотографировали его,
// вычислили его рост (1,75 м) и приблизительный возраст (33 г.).
// Все эти данные вместе с данными о содержимом его корзины (печенье и варенье) попали к нам в testDataType. Наша задача:
// — сформировать новое sync событие c командой «test.sales»
// — записать его в журнал PLog по смещению 10000 и в WLog по смещению 1000
// — записать характеристики этого покупателя в таблицу «test.photos» в новую запись
// — вычитать данные из журналов PLog и WLog, из таблицы и из вьюхи фотографий
//
/* Test scenario. */
// Someone who introduced himself as «Carloson 哇" 呀呀» was making a purchase at Supermarket # 1234.
// While he was uploading purchases at self-checkout # 762, the store's automated tools took a picture of him,
// calculated his height (1.75 m) and approximate age (33 years).
// All this data, along with the data on the contents of his basket (cookies and jam), came to us in testDataType. Our task:
// - form new sync event width command «test.sales»
// - write it to PLog at offset 10001 and in WLog at offset 1001
// - write the characteristics of this customer to the «test.photos» table into a new record
// - read the data from the PLog and WLog jounals, and from the «test.photo» table and from the «main.photoView» view
//

func TestBasicUsage(t *testing.T) {
	require := require.New(t)

	// create app configuration
	appConfigs := func() AppConfigsType {
		schemas := schemas.NewSchemaCache()

		saleParamsSchema := schemas.Add(istructs.NewQName("test", "Sale"), istructs.SchemaKind_ODoc)
		saleParamsSchema.
			AddField("Buyer", istructs.DataKind_string, true).
			AddField("Age", istructs.DataKind_int32, false).
			AddField("Height", istructs.DataKind_float32, false).
			AddField("isHuman", istructs.DataKind_bool, false).
			AddField("Photo", istructs.DataKind_bytes, false).
			AddContainer("Basket", istructs.NewQName("test", "Basket"), 1, 1)

		basketSchema := schemas.Add(istructs.NewQName("test", "Basket"), istructs.SchemaKind_ORecord)
		basketSchema.AddContainer("Good", istructs.NewQName("test", "Good"), 0, istructs.ContainerOccurs_Unbounded)

		goodSchema := schemas.Add(istructs.NewQName("test", "Good"), istructs.SchemaKind_ORecord)
		goodSchema.
			AddField("Name", istructs.DataKind_string, true).
			AddField("Code", istructs.DataKind_int64, true).
			AddField("Weight", istructs.DataKind_float64, false)

		saleSecurParamsSchema := schemas.Add(istructs.NewQName("test", "saleSecureArgs"), istructs.SchemaKind_Object)
		saleSecurParamsSchema.
			AddField("password", istructs.DataKind_string, true)

		docSchema := schemas.Add(istructs.NewQName("test", "photos"), istructs.SchemaKind_CDoc)
		docSchema.
			AddField("Buyer", istructs.DataKind_string, true).
			AddField("Age", istructs.DataKind_int32, false).
			AddField("Height", istructs.DataKind_float32, false).
			AddField("isHuman", istructs.DataKind_bool, false).
			AddField("Photo", istructs.DataKind_bytes, false)

		cfgs := make(AppConfigsType, 1)
		cfg := cfgs.AddConfig(istructs.AppQName_test1_app1, schemas)

		cfg.Resources.Add(
			NewCommandFunction(istructs.NewQName("test", "Sale"),
				istructs.NewQName("test", "Sale"), istructs.NewQName("test", "saleSecureArgs"), istructs.NullQName,
				NullCommandExec))

		return cfgs
	}()

	// gets AppStructProvider and AppStructs
	provider := Provide(appConfigs, iratesce.TestBucketsFactory, testTokensFactory(), simpleStorageProvder())

	app, err := provider.AppStructs(istructs.AppQName_test1_app1)
	require.NoError(err)

	// Build raw event demo
	// 1. gets event builder
	bld := app.Events().GetSyncRawEventBuilder(
		istructs.SyncRawEventBuilderParams{
			GenericRawEventBuilderParams: istructs.GenericRawEventBuilderParams{
				HandlingPartition: 55,
				PLogOffset:        10000,
				Workspace:         1234,
				WLogOffset:        1000,
				QName:             istructs.NewQName("test", "Sale"),
				RegisteredAt:      100500,
			},
			Device:   762,
			SyncedAt: 1005001,
		})

	// 2. make command params object
	cmd := bld.ArgumentObjectBuilder()

	cmd.PutRecordID(istructs.SystemField_ID, 1)
	cmd.PutString("Buyer", "Карлосон 哇\"呀呀") // to test unicode issues
	cmd.PutInt32("Age", 33)
	cmd.PutFloat32("Height", 1.75)
	cmd.PutBytes("Photo", []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 9, 8, 7, 6, 4, 4, 3, 2, 1, 0})

	basket := cmd.ElementBuilder("Basket")
	basket.PutRecordID(istructs.SystemField_ID, 2)

	good := basket.ElementBuilder("Good")
	good.PutRecordID(istructs.SystemField_ID, 3)
	good.PutString("Name", "Biscuits")
	good.PutInt64("Code", 7070)
	good.PutFloat64("Weight", 1.1)

	good = basket.ElementBuilder("Good")
	good.PutRecordID(istructs.SystemField_ID, 4)
	good.PutString("Name", "Jam")
	good.PutInt64("Code", 8080)
	good.PutFloat64("Weight", 2.02)

	security := bld.ArgumentUnloggedObjectBuilder()
	security.PutString("password", "12345")

	// 3. make result cuids
	cuids := bld.CUDBuilder()
	rec := cuids.Create(istructs.NewQName("test", "photos"))
	rec.PutRecordID(istructs.SystemField_ID, 1)
	rec.PutString("Buyer", "Карлосон 哇\"呀呀")
	rec.PutInt32("Age", 33)
	rec.PutFloat32("Height", 1.75)
	rec.PutBytes("Photo", []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 9, 8, 7, 6, 4, 4, 3, 2, 1, 0})

	// 4. get raw event
	rawEvent, buildErr := bld.BuildRawEvent()
	require.NoError(buildErr, buildErr)

	// Save raw event to PLog & WLog and save CUD demo
	// 5. save to PLog
	var nextID = istructs.FirstBaseRecordID
	pLogEvent, saveErr := app.Events().PutPlog(rawEvent, buildErr,
		func(tempId istructs.RecordID, _ istructs.ISchema) (storageID istructs.RecordID, err error) {
			storageID = nextID
			nextID++
			return storageID, nil
		},
	)
	require.NoError(saveErr, saveErr)
	defer pLogEvent.Release()

	// 6. save to WLog
	wLogEvent, err := app.Events().PutWlog(pLogEvent)
	require.NoError(err)
	defer wLogEvent.Release()

	// 7. save CUD
	err = app.Records().Apply(pLogEvent)
	require.NoError(err)

	// Read event from PLog & PLog and reads CUIDs demo
	// 8. read PLog
	var pLogEvent1 istructs.IPLogEvent
	_ = app.Events().ReadPLog(context.Background(), 55, 10000, 1,
		func(plogOffset istructs.Offset, event istructs.IPLogEvent) (err error) {
			pLogEvent1 = event
			return nil
		})

	require.NotNil(pLogEvent1)
	defer pLogEvent1.Release()

	// 9. read WLog
	var wLogEvent1 istructs.IWLogEvent
	_ = app.Events().ReadWLog(context.Background(), 1234, 1000, 1,
		func(wlogOffset istructs.Offset, event istructs.IWLogEvent) (err error) {
			wLogEvent1 = event
			return nil
		})

	require.NotNil(wLogEvent1)
	defer wLogEvent1.Release()
}

func TestBasicUsage_ViewRecords(t *testing.T) {
	require := require.New(t)

	appConfigs := func() AppConfigsType {
		schemas := schemas.NewSchemaCache()
		viewSchema := schemas.AddView(istructs.NewQName("test", "viewDrinks"))
		viewSchema.
			AddPartField("partitionKey1", istructs.DataKind_int64).
			AddClustColumn("clusteringColumn1", istructs.DataKind_int64).
			AddClustColumn("clusteringColumn2", istructs.DataKind_bool).
			AddClustColumn("clusteringColumn3", istructs.DataKind_string).
			AddValueField("id", istructs.DataKind_int64, true).
			AddValueField("name", istructs.DataKind_string, true).
			AddValueField("active", istructs.DataKind_bool, true)

		cfgs := make(AppConfigsType, 1)
		_ = cfgs.AddConfig(istructs.AppQName_test1_app1, schemas)

		return cfgs
	}

	p := Provide(appConfigs(), iratesce.TestBucketsFactory, testTokensFactory(), simpleStorageProvder())
	as, err := p.AppStructs(istructs.AppQName_test1_app1)
	require.NoError(err)
	viewRecords := as.ViewRecords()
	entries := []entryType{
		newEntry(viewRecords, 1, 100, true, "soda", 1, "Cola"),
		newEntry(viewRecords, 1, 100, true, "soda", 2, "Pepsi"), // dupe!
		newEntry(viewRecords, 1, 100, false, "soda", 3, "Sprite"),
		newEntry(viewRecords, 1, 200, false, "wine", 4, "White wine"),
		newEntry(viewRecords, 1, 200, true, "wine", 5, "Red wine"),
		newEntry(viewRecords, 2, 200, true, "wine", 4, "White wine"),
		newEntry(viewRecords, 2, 200, false, "wine", 5, "Red wine"),
	}
	for _, e := range entries {
		err := viewRecords.Put(e.wsid, e.key, e.value)
		require.NoError(err)
	}
	t.Run("Should read all records by WSID", func(t *testing.T) {
		kb := viewRecords.KeyBuilder(istructs.NewQName("test", "viewDrinks"))
		kb.PutInt64("partitionKey1", int64(1))
		counter := 0

		_ = viewRecords.Read(context.Background(), 1, kb, func(key istructs.IKey, value istructs.IValue) (err error) {
			counter++
			return nil
		})

		require.Equal(4, counter)
	})
	t.Run("Should read records by WSID and department", func(t *testing.T) {
		kb := viewRecords.KeyBuilder(istructs.NewQName("test", "viewDrinks"))
		kb.PutInt64("partitionKey1", 1)
		kb.PutInt64("clusteringColumn1", 200)
		counter := 0

		_ = viewRecords.Read(context.Background(), 1, kb, func(key istructs.IKey, value istructs.IValue) (err error) {
			counter++
			return nil
		})

		require.Equal(2, counter)
	})
	t.Run("Should read one record by WSID and department and active", func(t *testing.T) {
		kb := viewRecords.KeyBuilder(istructs.NewQName("test", "viewDrinks"))
		kb.PutInt64("partitionKey1", 2)
		kb.PutInt64("clusteringColumn1", 200)
		kb.PutBool("clusteringColumn2", true)
		counter := 0

		_ = viewRecords.Read(context.Background(), 2, kb, func(key istructs.IKey, value istructs.IValue) (err error) {
			counter++
			return nil
		})

		require.Equal(1, counter)
	})
	t.Run("Should read one record by WSID and department, active and code ignore wrong clustering columns order reason", func(t *testing.T) {
		kb := viewRecords.KeyBuilder(istructs.NewQName("test", "viewDrinks"))
		kb.PutInt64("partitionKey1", 2)
		kb.PutString("clusteringColumn3", "wine")
		kb.PutBool("clusteringColumn2", true)
		kb.PutInt64("clusteringColumn1", 200)
		counter := 0

		_ = viewRecords.Read(context.Background(), 2, kb, func(key istructs.IKey, value istructs.IValue) (err error) {
			counter++
			return nil
		})

		require.Equal(1, counter)
	})
}

// TestBasicUsage_Resources: Demonstrates basic usage resources
func TestBasicUsage_Resources(t *testing.T) {
	require := require.New(t)
	test := test()

	// gets AppStructProvider and AppStructs
	provider := Provide(test.AppConfigs, iratesce.TestBucketsFactory, testTokensFactory(), simpleStorageProvder())

	app, err := provider.AppStructs(test.appName)
	require.NoError(err)

	t.Run("Basic usage NewCommandFunction", func(t *testing.T) {
		funcQName := istructs.NewQName("testpkg", "cfunc")
		paramsSchema := istructs.NewQName("testpkg", "cfuncParams")
		resultSchema := istructs.NullQName

		f := NewCommandFunction(funcQName, paramsSchema, istructs.NullQName, resultSchema, NullCommandExec)
		require.Equal(funcQName, f.QName())
		require.Equal(istructs.ResourceKind_CommandFunction, f.Kind())
		require.Equal(paramsSchema, f.ParamsSchema())
		require.Equal(istructs.NullQName, f.UnloggedParamsSchema())
		require.Equal(resultSchema, f.ResultSchema())

		// Calls have no effect since we use Null* closures

		f.Exec(istructs.ExecCommandArgs{})

		// Test String()
		log.Println(f)
	})

	t.Run("Basic usage NewQueryFunction", func(t *testing.T) {
		myExecQuery := func(ctx context.Context, qf istructs.IQueryFunction, args istructs.ExecQueryArgs, callback istructs.ExecQueryCallback) error {
			// Can use NullExecQuery instead of myExecQuery, it does nothing
			NullQueryExec(ctx, qf, args, callback)

			callback(&istructs.NullObject{})
			return nil
		}

		funcQName := istructs.NewQName("testpkg", "qfunc")
		paramsSchema := istructs.NewQName("testpkg", "qfuncParams")
		resultSchema := istructs.NullQName

		f := NewQueryFunction(funcQName, paramsSchema, resultSchema, myExecQuery)
		require.Equal(funcQName, f.QName())
		require.Equal(istructs.ResourceKind_QueryFunction, f.Kind())
		require.Equal(paramsSchema, f.ParamsSchema())
		require.Equal(resultSchema, f.ResultSchema(istructs.PrepareArgs{})) // ???

		// Depends on myExecQuery
		f.Exec(context.Background(), istructs.ExecQueryArgs{}, func(istructs.IObject) error { return nil })

		// Test String()
		log.Println(f)
	})

	t.Run("test app.Resources()", func(t *testing.T) {
		r := app.Resources().QueryResource(test.queryPhotoFunctionName)
		require.NotNil(r)

		bld := app.Resources().QueryFunctionArgsBuilder(r.(istructs.IQueryFunction))
		require.NotNil(bld)
		bld.PutString(test.buyerIdent, test.buyerValue)
		doc, err := bld.Build()
		require.NoError(err)
		require.NotNil(doc)
	})
}

// TestBasicUsage_Schemas: Demonstrates basic usage schemas
func TestBasicUsage_Schemas(t *testing.T) {
	require := require.New(t)
	test := test()

	// gets AppStructProvider and AppStructs
	provider := Provide(test.AppConfigs, iratesce.TestBucketsFactory, testTokensFactory(), simpleStorageProvder())

	app, err := provider.AppStructs(test.appName)
	require.NoError(err)

	t.Run("I. test top level schema (command object)", func(t *testing.T) {
		schema := app.Schemas().Schema(test.saleCmdDocName)

		require.NotNil(schema)
		require.Equal(istructs.SchemaKind_ODoc, schema.Kind())

		// check fields
		fields := make(map[string]istructs.DataKindType)
		schema.Fields(func(fn string, kind istructs.DataKindType) {
			fields[fn] = kind
		})
		require.Equal(7, len(fields)) // 2 system {sys.QName, sys.ID} + 5 user
		require.Equal(istructs.DataKind_string, fields[test.buyerIdent])
		require.Equal(istructs.DataKind_int32, fields[test.ageIdent])
		require.Equal(istructs.DataKind_float32, fields[test.heightIdent])
		require.Equal(istructs.DataKind_bool, fields[test.humanIdent])
		require.Equal(istructs.DataKind_bytes, fields[test.photoIdent])

		schema.Containers(func(name string, sch istructs.QName) {
			require.Equal(test.basketIdent, name)
			require.Equal(istructs.NewQName(test.pkgName, test.basketIdent), sch)
			t.Run("II. test first level nested schema (basket)", func(t *testing.T) {
				schema := app.Schemas().Schema(istructs.NewQName(test.pkgName, test.basketIdent))
				require.NotNil(schema)
				require.Equal(istructs.SchemaKind_ORecord, schema.Kind())

				schema.Containers(func(name string, sch istructs.QName) {
					require.Equal(test.goodIdent, name)
					require.Equal(istructs.NewQName(test.pkgName, test.goodIdent), sch)

					t.Run("III. test second level nested schema (good)", func(t *testing.T) {
						schema := app.Schemas().Schema(istructs.NewQName(test.pkgName, test.goodIdent))
						require.NotNil(schema)
						require.Equal(istructs.SchemaKind_ORecord, schema.Kind())

						fields := make(map[string]istructs.DataKindType)
						schema.Fields(func(fn string, kind istructs.DataKindType) {
							fields[fn] = kind
						})
						require.Equal(8, len(fields)) // 4 system {sys.QName, sys.ID, sys.ParentID, sys.Container} + 4 user
						require.Equal(istructs.DataKind_RecordID, fields[test.saleIdent])
						require.Equal(istructs.DataKind_string, fields[test.nameIdent])
						require.Equal(istructs.DataKind_int64, fields[test.codeIdent])
						require.Equal(istructs.DataKind_float64, fields[test.weightIdent])
					})
				})
			})
		})
	})
}

func Test_BasicUsageDescribePackages(t *testing.T) {

	require := require.New(t)

	app := func() istructs.IAppStructs {
		schemas := schemas.NewSchemaCache()

		recSchema := schemas.Add(istructs.NewQName("types", "CRec"), istructs.SchemaKind_CRecord)
		recSchema.AddField("int", istructs.DataKind_int64, false)

		docQName := istructs.NewQName("types", "CDoc")
		docSchema := schemas.Add(docQName, istructs.SchemaKind_CDoc)
		docSchema.AddField("str", istructs.DataKind_string, true)
		docSchema.AddField("fld", istructs.DataKind_int32, true)

		docSchema.AddContainer("rec", recSchema.QName(), 0, istructs.ContainerOccurs_Unbounded)

		viewSchema := schemas.AddView(istructs.NewQName("types", "View"))
		viewSchema.AddPartField("int", istructs.DataKind_int64)
		viewSchema.AddClustColumn("str", istructs.DataKind_string)
		viewSchema.AddValueField("bool", istructs.DataKind_bool, false)

		argSchema := schemas.Add(istructs.NewQName("types", "Arg"), istructs.SchemaKind_Object)
		argSchema.AddField("bool", istructs.DataKind_bool, false)

		cfgs := make(AppConfigsType)
		cfg := cfgs.AddConfig(istructs.AppQName_test1_app1, schemas)

		cfg.Resources.Add(
			NewCommandFunction(
				istructs.NewQName("commands", "cmd"),
				argSchema.QName(),
				istructs.NullQName,
				docSchema.QName(),
				NullCommandExec))

		qNameQry := istructs.NewQName("commands", "query")
		cfg.Resources.Add(
			NewQueryFunction(
				qNameQry,
				argSchema.QName(),
				viewSchema.Name(),
				NullQueryExec))

		cfg.Uniques.Add(docQName, []string{"str"})
		cfg.Uniques.Add(docQName, []string{"str", "fld"})

		cfg.FunctionRateLimits.AddAppLimit(qNameQry, istructs.RateLimit{
			Period:                1,
			MaxAllowedPerDuration: 2,
		})
		cfg.FunctionRateLimits.AddWorkspaceLimit(qNameQry, istructs.RateLimit{
			Period:                3,
			MaxAllowedPerDuration: 4,
		})

		provider := Provide(cfgs, iratesce.TestBucketsFactory, testTokensFactory(), simpleStorageProvder())
		app, err := provider.AppStructs(istructs.AppQName_test1_app1)
		require.NoError(err)

		return app
	}()

	pkgNames := app.DescribePackageNames()
	require.NotNil(pkgNames)
	require.EqualValues(2, len(pkgNames))

	for _, name := range pkgNames {
		pkg := app.DescribePackage(name)
		require.NotNil(pkg)

		bytes, err := json.Marshal(pkg)
		require.NoError(err)

		logger.Info("package: ", name)
		logger.Info(string(bytes))
	}
}

func Test_Provide(t *testing.T) {
	require := require.New(t)
	test := test()

	t.Run("AppStructs() must error if unknown app name", func(t *testing.T) {
		cfgs := make(AppConfigsType)
		cfgs.AddConfig(istructs.AppQName_test1_app1, schemas.NewSchemaCache())
		p := Provide(cfgs, iratesce.TestBucketsFactory, testTokensFactory(), nil)
		require.NotNil(p)

		_, err := p.AppStructs(istructs.NewAppQName("test1", "unknownApp"))
		require.ErrorIs(err, istructs.ErrAppNotFound)
	})

	t.Run("check application ClusterAppID() and AppQName()", func(t *testing.T) {
		provider := Provide(test.AppConfigs, iratesce.TestBucketsFactory, testTokensFactory(), simpleStorageProvder())

		app, err := provider.AppStructs(test.appName)
		require.NoError(err)

		require.NotNil(app)

		require.Equal(istructs.ClusterAppID_test1_app1, app.ClusterAppID())
		require.Equal(istructs.AppQName_test1_app1, app.AppQName())
	})
}