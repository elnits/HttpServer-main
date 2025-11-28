package database

import "fmt"

const defaultExportBatchSize = 500

// StreamConstantsByUpload читает константы выгрузки порциями и передает батчи в handler.
func (db *DB) StreamConstantsByUpload(uploadID int, batchSize int, handler func([]*Constant) error) error {
	if handler == nil {
		return fmt.Errorf("handler is required")
	}
	if batchSize <= 0 {
		batchSize = defaultExportBatchSize
	}

	offset := 0
	for {
		batch, err := db.getConstantsBatch(uploadID, offset, batchSize)
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}
		if err := handler(batch); err != nil {
			return err
		}
		if len(batch) < batchSize {
			return nil
		}
		offset += len(batch)
	}
}

// StreamCatalogsByUpload читает метаданные справочников (с фильтрацией по именам) и передает их обработчику.
func (db *DB) StreamCatalogsByUpload(uploadID int, catalogNames []string, handler func(*Catalog) error) error {
	if handler == nil {
		return fmt.Errorf("handler is required")
	}

	catalogs, err := db.getCatalogsFiltered(uploadID, catalogNames)
	if err != nil {
		return err
	}

	for _, catalog := range catalogs {
		if err := handler(catalog); err != nil {
			return err
		}
	}
	return nil
}

// StreamCatalogItems читает элементы справочников выгрузки батчами и передает в handler.
func (db *DB) StreamCatalogItems(uploadID int, catalogNames []string, batchSize int, handler func([]*CatalogItem) error) error {
	if handler == nil {
		return fmt.Errorf("handler is required")
	}
	if batchSize <= 0 {
		batchSize = defaultExportBatchSize
	}

	offset := 0
	for {
		items, err := db.getCatalogItemsBatch(uploadID, catalogNames, offset, batchSize)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		if err := handler(items); err != nil {
			return err
		}
		if len(items) < batchSize {
			return nil
		}
		offset += len(items)
	}
}

// StreamNomenclatureItems читает номенклатуру с характеристиками батчами.
func (db *DB) StreamNomenclatureItems(uploadID int, batchSize int, handler func([]*NomenclatureItem) error) error {
	if handler == nil {
		return fmt.Errorf("handler is required")
	}
	if batchSize <= 0 {
		batchSize = defaultExportBatchSize
	}

	offset := 0
	for {
		items, err := db.getNomenclatureBatch(uploadID, offset, batchSize)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		if err := handler(items); err != nil {
			return err
		}
		if len(items) < batchSize {
			return nil
		}
		offset += len(items)
	}
}

func (db *DB) getConstantsBatch(uploadID int, offset, limit int) ([]*Constant, error) {
	query := `
		SELECT id, upload_id, name, synonym, type, value, created_at
		FROM constants
		WHERE upload_id = ?
		ORDER BY id
		LIMIT ? OFFSET ?
	`

	rows, err := db.conn.Query(query, uploadID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get constants batch: %w", err)
	}
	defer rows.Close()

	var result []*Constant
	for rows.Next() {
		item := &Constant{}
		if err := rows.Scan(&item.ID, &item.UploadID, &item.Name, &item.Synonym, &item.Type, &item.Value, &item.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan constant: %w", err)
		}
		result = append(result, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating constants batch: %w", err)
	}

	return result, nil
}

func (db *DB) getCatalogsFiltered(uploadID int, catalogNames []string) ([]*Catalog, error) {
	baseQuery := `
		SELECT id, upload_id, name, synonym, created_at
		FROM catalogs
		WHERE upload_id = ?
	`

	args := []interface{}{uploadID}
	if len(catalogNames) > 0 {
		baseQuery += " AND name IN ("
		for i := range catalogNames {
			if i > 0 {
				baseQuery += ","
			}
			baseQuery += "?"
		}
		baseQuery += ")"
		for _, name := range catalogNames {
			args = append(args, name)
		}
	}

	baseQuery += " ORDER BY name"

	rows, err := db.conn.Query(baseQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get catalogs: %w", err)
	}
	defer rows.Close()

	var catalogs []*Catalog
	for rows.Next() {
		catalog := &Catalog{}
		if err := rows.Scan(&catalog.ID, &catalog.UploadID, &catalog.Name, &catalog.Synonym, &catalog.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan catalog: %w", err)
		}
		catalogs = append(catalogs, catalog)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating catalogs: %w", err)
	}

	return catalogs, nil
}

func (db *DB) getCatalogItemsBatch(uploadID int, catalogNames []string, offset, limit int) ([]*CatalogItem, error) {
	items, _, err := db.GetCatalogItemsByUpload(uploadID, catalogNames, offset, limit)
	return items, err
}

func (db *DB) getNomenclatureBatch(uploadID int, offset, limit int) ([]*NomenclatureItem, error) {
	query := `
		SELECT id, upload_id, nomenclature_reference, nomenclature_code, nomenclature_name,
		       characteristic_reference, characteristic_name, 
		       COALESCE(attributes_xml, '') as attributes_xml,
		       COALESCE(table_parts_xml, '') as table_parts_xml,
		       created_at
		FROM nomenclature_items
		WHERE upload_id = ?
		ORDER BY id
		LIMIT ? OFFSET ?
	`

	rows, err := db.conn.Query(query, uploadID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get nomenclature batch: %w", err)
	}
	defer rows.Close()

	var items []*NomenclatureItem
	for rows.Next() {
		item := &NomenclatureItem{}
		if err := rows.Scan(
			&item.ID,
			&item.UploadID,
			&item.NomenclatureReference,
			&item.NomenclatureCode,
			&item.NomenclatureName,
			&item.CharacteristicReference,
			&item.CharacteristicName,
			&item.AttributesXML,
			&item.TablePartsXML,
			&item.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan nomenclature item: %w", err)
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating nomenclature batch: %w", err)
	}

	return items, nil
}


