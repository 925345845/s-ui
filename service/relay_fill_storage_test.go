package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Hhz0823/1s-ui/database"
	"github.com/Hhz0823/1s-ui/database/model"
	"github.com/xuri/excelize/v2"
	"gorm.io/gorm"
)

func TestRelayFillStorageAtomicAppend(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SUI_DB_FOLDER", dir)
	if err := database.InitDB(filepath.Join(dir, "fill-storage.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	first := model.RelayItem{SourceRow: 2, InboundTag: "first", ListenPort: 30000, IPv6: "2001:db8::1", Username: "first", Password: "secret1", Protocol: "socks", Export: "192.0.2.1:30000:first:secret1", AppleIDIPv4Only: true}
	second := model.RelayItem{SourceRow: 1, InboundTag: "second", ListenPort: 30005, IPv6: "2001:db8::2", Username: "second", Password: "secret2", Protocol: "socks", Export: "192.0.2.1:30005:second:secret2", AppleIDIPv4Only: true}
	pool := model.RelayPool{Name: "one-task", Mode: relayModePaired, Protocol: "socks", ListenHost: "192.0.2.1", PortStart: 30000, Count: 1, Items: mustJSON([]model.RelayItem{first}), RotationEnabled: true, RotationIntervalMinutes: 60, NextRotateAt: 999}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := saveRelayFillRound(tx, &pool, []model.RelayItem{first}, 0); err != nil {
			return err
		}
		return ensureRelayRefreshLinksTx(tx, pool.Id, []model.RelayItem{first})
	}); err != nil {
		t.Fatal(err)
	}
	var originalLink model.RelayRefreshLink
	if err := db.First(&originalLink).Error; err != nil {
		t.Fatal(err)
	}
	round := pool
	round.Id, round.PortStart, round.Items = 0, second.ListenPort, mustJSON([]model.RelayItem{second})
	appendRound := func(tx *gorm.DB) error {
		if err := saveRelayFillRound(tx, &round, []model.RelayItem{second}, pool.Id); err != nil {
			return err
		}
		return ensureRelayRefreshLinksTx(tx, round.Id, []model.RelayItem{second})
	}
	// A later config failure must roll back both the appended items and links.
	failure := errors.New("simulated configuration failure")
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := appendRound(tx); err != nil {
			return err
		}
		return failure
	}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	var saved model.RelayPool
	if err := db.First(&saved, pool.Id).Error; err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Model(&model.RelayRefreshLink{}).Count(&count)
	if saved.Count != 1 || !bytes.Equal(saved.Items, pool.Items) || count != 1 {
		t.Fatal("rollback changed saved pool or retained new links")
	}
	if err := db.Transaction(appendRound); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&saved, pool.Id).Error; err != nil {
		t.Fatal(err)
	}
	var items []model.RelayItem
	if err := json.Unmarshal(saved.Items, &items); err != nil {
		t.Fatal(err)
	}
	if saved.Count != 2 || !reflect.DeepEqual(items, []model.RelayItem{first, second}) || saved.PortStart != 30000 || !saved.RotationEnabled || saved.NextRotateAt != 999 {
		t.Fatalf("append changed pool metadata: %+v", saved)
	}
	db.Model(&model.RelayPool{}).Count(&count)
	if count != 1 {
		t.Fatal("append created another pool")
	}
	var link model.RelayRefreshLink
	if err := db.First(&link, originalLink.Id).Error; err != nil || link.Token != originalLink.Token {
		t.Fatal("original refresh token changed")
	}
	withTokens := []model.RelayPool{saved}
	if err := (&ConfigService{}).populateRelayRefreshTokens(withTokens); err != nil {
		t.Fatal(err)
	}
	xlsx, err := buildRelayBitBrowserWorkbook(withTokens[0], "https://panel.example/app/")
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(bytes.NewReader(xlsx))
	if err != nil {
		t.Fatal(err)
	}
	defer book.Close()
	rows, err := book.GetRows(book.GetSheetName(0))
	if err != nil || len(rows) != 5 {
		t.Fatalf("unified export must include both nodes: rows=%d err=%v", len(rows), err)
	}
	if rows[3][5] != first.Export || rows[4][5] != second.Export || rows[3][9] != "https://panel.example/app/refresh/"+originalLink.Token {
		t.Fatal("unified export lost credentials or existing refresh link")
	}
	// Never silently re-create a deleted destination or duplicate an existing row.
	if err := db.Transaction(appendRound); err == nil {
		t.Fatal("duplicate append accepted")
	}
	if err := db.Delete(&model.RelayPool{}, pool.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(appendRound); err == nil {
		t.Fatal("missing destination was silently recreated")
	}
}
