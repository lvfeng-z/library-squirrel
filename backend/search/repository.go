package search

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	dto2 "github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"

	"gorm.io/gorm"
)

// SearchRepository 搜索仓储实现
type SearchRepository struct {
	db *gorm.DB
}

// NewRepository 创建搜索仓储
func NewRepository(db *gorm.DB) *SearchRepository {
	return &SearchRepository{db: db}
}

// QuerySearchConditionPage 查询搜索条件分页
func (r *SearchRepository) QuerySearchConditionPage(ctx context.Context, page, pageSize int, keyword string, types []dto2.SearchType) ([]*dto2.SelectItem, int64, error) {
	var statements []string
	var countStatements []string
	var params []interface{}

	// 关键词模糊匹配
	hasKeyword := keyword != ""
	var likePattern string
	if hasKeyword {
		likePattern = "%" + keyword + "%"
	}

	// 判断是否包含某种类型（或全部类型）
	includeLocalTag := len(types) == 0 || containsType(types, dto2.LocalTag)
	includeSiteTag := len(types) == 0 || containsType(types, dto2.SiteTag)
	includeLocalAuthor := len(types) == 0 || containsType(types, dto2.LocalAuthor)
	includeSiteAuthor := len(types) == 0 || containsType(types, dto2.SiteAuthor)

	// 本地标签查询
	if includeLocalTag {
		stmt := `SELECT id || 'localTag' AS value, local_tag_name AS label, last_use AS lastUse,
				 JSON_OBJECT('type', 1, 'id', id) AS extraData
				 FROM local_tag`
		countStmt := `SELECT COUNT(*) FROM local_tag`
		if hasKeyword {
			stmt += ` WHERE local_tag_name LIKE ?`
			countStmt += ` WHERE local_tag_name LIKE ?`
			params = append(params, likePattern)
		}
		statements = append(statements, stmt)
		countStatements = append(countStatements, countStmt)
	}

	// 站点标签查询
	if includeSiteTag {
		stmt := `SELECT t1.id || 'siteTag' AS value, t1.site_tag_name AS label, t1.last_use AS lastUse,
				 JSON_OBJECT('type', 2, 'id', t1.id,
				 'localTag', JSON_OBJECT('id', COALESCE(t2.id, 0), 'localTagName', COALESCE(t2.local_tag_name, ''), 'baseLocalTagId', COALESCE(t2.base_local_tag_id, 0)),
				 'site', JSON_OBJECT('id', COALESCE(t3.id, 0), 'siteName', COALESCE(t3.site_name, ''), 'siteDescription', COALESCE(t3.site_description, '')),
				 'namespace', t1.namespace
				 ) AS extraData
				 FROM site_tag t1
				 LEFT JOIN local_tag t2 ON t1.local_tag_id = t2.id
				 LEFT JOIN site t3 ON t1.site_id = t3.id`
		countStmt := `SELECT COUNT(*) FROM site_tag t1 LEFT JOIN local_tag t2 ON t1.local_tag_id = t2.id LEFT JOIN site t3 ON t1.site_id = t3.id`
		if hasKeyword {
			stmt += ` WHERE t1.site_tag_name LIKE ?`
			countStmt += ` WHERE t1.site_tag_name LIKE ?`
			params = append(params, likePattern)
		}
		statements = append(statements, stmt)
		countStatements = append(countStatements, countStmt)
	}

	// 本地作者查询
	if includeLocalAuthor {
		stmt := `SELECT id || 'localAuthor' AS value, author_name AS label, last_use AS lastUse,
				 JSON_OBJECT('type', 3, 'id', id) AS extraData
				 FROM local_author`
		countStmt := `SELECT COUNT(*) FROM local_author`
		if hasKeyword {
			stmt += ` WHERE author_name LIKE ?`
			countStmt += ` WHERE author_name LIKE ?`
			params = append(params, likePattern)
		}
		statements = append(statements, stmt)
		countStatements = append(countStatements, countStmt)
	}

	// 站点作者查询
	if includeSiteAuthor {
		stmt := `SELECT t1.id || 'siteAuthor' AS value, t1.author_name AS label, t1.last_use AS lastUse,
				 JSON_OBJECT('type', 4, 'id', t1.id,
				 'siteAuthor', JSON_OBJECT('id', COALESCE(t2.id, 0), 'authorName', COALESCE(t2.author_name, '')),
				 'site', JSON_OBJECT('id', COALESCE(t3.id, 0), 'siteName', COALESCE(t3.site_name, ''), 'siteDescription', COALESCE(t3.site_description, ''))
				 ) AS extraData
				 FROM site_author t1
				 LEFT JOIN local_author t2 ON t1.local_author_id = t2.id
				 LEFT JOIN site t3 ON t1.site_id = t3.id`
		countStmt := `SELECT COUNT(*) FROM site_author t1 LEFT JOIN local_author t2 ON t1.local_author_id = t2.id LEFT JOIN site t3 ON t1.site_id = t3.id`
		if hasKeyword {
			stmt += ` WHERE t1.author_name LIKE ?`
			countStmt += ` WHERE t1.author_name LIKE ?`
			params = append(params, likePattern)
		}
		statements = append(statements, stmt)
		countStatements = append(countStatements, countStmt)
	}

	if len(statements) == 0 {
		return []*dto2.SelectItem{}, 0, nil
	}

	// 构建联合查询 - 使用完整查询语句（不带括号），与旧主进程保持一致
	unionStatement := strings.Join(statements, " UNION ALL ")

	// 计算总数
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM (SELECT * FROM (%s)) AS total", unionStatement)
	err := r.db.WithContext(ctx).Raw(countQuery, params...).Scan(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	query := fmt.Sprintf("SELECT value, label, extraData FROM (%s ORDER BY lastUse DESC LIMIT %d OFFSET %d)", unionStatement, pageSize, offset)

	rows, err := r.db.WithContext(ctx).Raw(query, params...).Rows()
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []*dto2.SelectItem
	for rows.Next() {
		var item dto2.SelectItem
		var extraData sql.NullString

		err := rows.Scan(&item.Value, &item.Label, &extraData)
		if err != nil {
			return nil, 0, err
		}

		// 解析 extraData JSON
		if extraData.Valid && extraData.String != "" {
			var parsedExtra interface{}
			if err := json.Unmarshal([]byte(extraData.String), &parsedExtra); err == nil {
				item.ExtraData = parsedExtra
			}
		}

		// 设置 subLabels
		if item.ExtraData != nil {
			if extraMap, ok := item.ExtraData.(map[string]interface{}); ok {
				subLabels := []string{}
				if typeVal, ok := extraMap["type"].(float64); ok {
					switch dto2.SearchType(typeVal) {
					case dto2.LocalTag:
						subLabels = append(subLabels, "tag", "local")
					case dto2.SiteTag:
						subLabels = append(subLabels, "tag")
						if siteMap, ok := extraMap["site"].(map[string]interface{}); ok {
							if siteName, ok := siteMap["siteName"].(string); ok && siteName != "" {
								subLabels = append(subLabels, siteName)
							} else {
								subLabels = append(subLabels, "?")
							}
						}
					case dto2.LocalAuthor:
						subLabels = append(subLabels, "author", "local")
					case dto2.SiteAuthor:
						subLabels = append(subLabels, "author")
						if siteMap, ok := extraMap["site"].(map[string]interface{}); ok {
							if siteName, ok := siteMap["siteName"].(string); ok && siteName != "" {
								subLabels = append(subLabels, siteName)
							} else {
								subLabels = append(subLabels, "?")
							}
						}
					}
				}
				item.SubLabels = subLabels
			}
		}

		results = append(results, &item)
	}

	return results, total, nil
}

// QueryWorkPage 查询作品分页
func (r *SearchRepository) QueryWorkPage(ctx context.Context, page, pageSize int, conditions []*dto2.SearchCondition) ([]*dto2.WorkFullDTO, int64, error) {
	// 构建 WHERE 子句
	whereClause, params := buildWhereClause(conditions)

	// 计算总数
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(DISTINCT t1.id) FROM work t1 %s", whereClause)
	if err := r.db.WithContext(ctx).Raw(countQuery, params...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询 — SQL 子查询产出的 JSON 结构与 WorkFullDTO 中各子 DTO 类型对齐，可直接 json.Unmarshal
	offset := (page - 1) * pageSize
	query := fmt.Sprintf(`
		SELECT t1.id, t1.create_time, t1.update_time, t1.site_id, t1.site_work_id, t1.site_work_name,
				t1.site_author_id, t1.site_work_description, t1.site_upload_time, t1.site_update_time,
				t1.nick_name, t1.local_author_id, t1.last_view,
			(SELECT JSON_OBJECT(
				'id', r.id, 'workId', r.work_id, 'taskId', r.task_id,
				'suggestName', r.suggest_name, 'resourceType', r.resource_type, 'resourceComplete', r.resource_complete,
				'stores', (SELECT JSON_GROUP_ARRAY(JSON_OBJECT(
					'storeType', rs.store_type, 'generation', rs.generation,
					'store', JSON_OBJECT(
						'id', ps.id, 'filePath', ps.file_path, 'fileName', ps.file_name,
						'filenameExtension', ps.filename_extension, 'completedAt', ps.completed_at,
						'createTime', ps.create_time, 'updateTime', ps.update_time)))
					FROM resource_store rs
					LEFT JOIN persistent_store ps ON rs.store_id = ps.id
					WHERE rs.resource_id = r.id),
				'workStore', COALESCE(
					(SELECT JSON_OBJECT('id', ps.id, 'filePath', ps.file_path, 'fileName', ps.file_name, 'filenameExtension', ps.filename_extension, 'completedAt', ps.completed_at, 'createTime', ps.create_time, 'updateTime', ps.update_time) FROM resource_store rs INNER JOIN persistent_store ps ON rs.store_id = ps.id WHERE rs.resource_id = r.id AND rs.store_type = 'videoMain' LIMIT 1),
					(SELECT JSON_OBJECT('id', ps.id, 'filePath', ps.file_path, 'fileName', ps.file_name, 'filenameExtension', ps.filename_extension, 'completedAt', ps.completed_at, 'createTime', ps.create_time, 'updateTime', ps.update_time) FROM resource_store rs INNER JOIN persistent_store ps ON rs.store_id = ps.id WHERE rs.resource_id = r.id AND rs.store_type = 'image' LIMIT 1),
					(SELECT JSON_OBJECT('id', ps.id, 'filePath', ps.file_path, 'fileName', ps.file_name, 'filenameExtension', ps.filename_extension, 'completedAt', ps.completed_at, 'createTime', ps.create_time, 'updateTime', ps.update_time) FROM resource_store rs INNER JOIN persistent_store ps ON rs.store_id = ps.id WHERE rs.resource_id = r.id AND rs.store_type = 'document' LIMIT 1),
					(SELECT JSON_OBJECT('id', ps.id, 'filePath', ps.file_path, 'fileName', ps.file_name, 'filenameExtension', ps.filename_extension, 'completedAt', ps.completed_at, 'createTime', ps.create_time, 'updateTime', ps.update_time) FROM resource_store rs INNER JOIN persistent_store ps ON rs.store_id = ps.id WHERE rs.resource_id = r.id AND rs.store_type = 'videoTrack' LIMIT 1)
				),
				'thumbnailStore', (SELECT JSON_OBJECT(
						'id', ps.id, 'filePath', ps.file_path, 'fileName', ps.file_name,
						'filenameExtension', ps.filename_extension, 'completedAt', ps.completed_at,
						'createTime', ps.create_time, 'updateTime', ps.update_time)
					FROM resource_store rs
					INNER JOIN persistent_store ps ON rs.store_id = ps.id
					WHERE rs.resource_id = r.id AND rs.store_type = 'thumbnail' LIMIT 1),
				'createTime', r.create_time, 'updateTime', r.update_time)
			FROM resource r
			WHERE t1.id = r.work_id
			LIMIT 1) AS resource,
			(SELECT JSON_GROUP_ARRAY(JSON_OBJECT(
				'id', lt.id, 'localTagName', lt.local_tag_name, 'baseLocalTagId', lt.base_local_tag_id,
				'description', lt.description, 'lastUse', lt.last_use,
				'createTime', lt.create_time, 'updateTime', lt.update_time))
			FROM re_work_tag rwt
			INNER JOIN local_tag lt ON rwt.local_tag_id = lt.id
			WHERE t1.id = rwt.work_id) AS localTags,
			(SELECT JSON_GROUP_ARRAY(JSON_OBJECT(
				'siteTag', JSON_OBJECT(
					'id', st.id, 'siteId', st.site_id, 'siteTagId', st.site_tag_id, 'siteTagName', st.site_tag_name,
					'baseSiteTagId', st.base_site_tag_id, 'description', st.description, 'localTagId', st.local_tag_id,
					'lastUse', st.last_use, 'createTime', st.create_time, 'updateTime', st.update_time),
				'localTag', CASE WHEN lt.id IS NOT NULL THEN JSON_OBJECT(
					'id', lt.id, 'localTagName', lt.local_tag_name, 'baseLocalTagId', lt.base_local_tag_id,
					'description', lt.description, 'lastUse', lt.last_use,
					'createTime', lt.create_time, 'updateTime', lt.update_time)
				END,
				'site', CASE WHEN s.id IS NOT NULL THEN JSON_OBJECT(
					'id', s.id, 'siteName', s.site_name, 'siteDescription', s.site_description)
				END))
			FROM re_work_tag rwt
			INNER JOIN site_tag st ON rwt.site_tag_id = st.id
			LEFT JOIN local_tag lt ON st.local_tag_id = lt.id
			LEFT JOIN site s ON st.site_id = s.id
			WHERE t1.id = rwt.work_id) AS siteTags,
			(SELECT JSON_GROUP_ARRAY(JSON_OBJECT(
				'id', la.id, 'authorName', la.author_name, 'introduce', la.introduce,
				'lastUse', la.last_use, 'createTime', la.create_time, 'updateTime', la.update_time))
			FROM re_work_author rwa
			INNER JOIN local_author la ON rwa.local_author_id = la.id
			WHERE t1.id = rwa.work_id) AS localAuthors,
			(SELECT JSON_GROUP_ARRAY(JSON_OBJECT(
				'siteAuthor', JSON_OBJECT(
					'id', sa.id, 'siteId', sa.site_id, 'siteAuthorId', sa.site_author_id, 'authorName', sa.author_name,
					'fixedAuthorName', sa.fixed_author_name, 'siteAuthorNameBefore', sa.site_author_name_before,
					'introduce', sa.introduce, 'homepage', sa.homepage, 'localAuthorId', sa.local_author_id,
					'lastUse', sa.last_use, 'createTime', sa.create_time, 'updateTime', sa.update_time),
				'localAuthor', CASE WHEN la.id IS NOT NULL THEN JSON_OBJECT(
					'id', la.id, 'authorName', la.author_name, 'introduce', la.introduce,
					'lastUse', la.last_use, 'createTime', la.create_time, 'updateTime', la.update_time)
				END,
				'site', CASE WHEN s.id IS NOT NULL THEN JSON_OBJECT(
					'id', s.id, 'siteName', s.site_name, 'siteDescription', s.site_description)
				END))
			FROM re_work_author rwa
			INNER JOIN site_author sa ON rwa.site_author_id = sa.id
			LEFT JOIN local_author la ON sa.local_author_id = la.id
			LEFT JOIN site s ON sa.site_id = s.id
			WHERE t1.id = rwa.work_id) AS siteAuthors
		FROM work t1
		%s
		GROUP BY t1.id
		ORDER BY t1.update_time DESC
		LIMIT %d OFFSET %d
	`, whereClause, pageSize, offset)

	rows, err := r.db.WithContext(ctx).Raw(query, params...).Rows()
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []*dto2.WorkFullDTO
	for rows.Next() {
		work := entity2.NewWork()
		var resource, localTags, siteTags, localAuthors, siteAuthors sql.NullString

		if err := rows.Scan(
			&work.ID, &work.CreateTime, &work.UpdateTime, &work.SiteID, &work.SiteWorkID, &work.SiteWorkName,
			&work.SiteAuthorID, &work.SiteWorkDescription, &work.SiteUploadTime, &work.SiteUpdateTime,
			&work.NickName, &work.LocalAuthorID, &work.LastView,
			&resource, &localTags, &siteTags, &localAuthors, &siteAuthors,
		); err != nil {
			return nil, 0, err
		}

		dto := dto2.NewWorkFullDTO(work)

		// 解析 resource
		if resource.Valid && resource.String != "" && resource.String != "null" {
			var resFull *dto2.ResourceFullDTO
			if json.Unmarshal([]byte(resource.String), &resFull) == nil {
				dto.Resource = resFull
			}
		}

		// 解析 localTags
		if localTags.Valid && localTags.String != "" && localTags.String != "null" {
			var tags []*sdkdto.LocalTagDTO
			if json.Unmarshal([]byte(localTags.String), &tags) == nil {
				dto.LocalTags = tags
			}
		}

		// 解析 siteTags
		if siteTags.Valid && siteTags.String != "" && siteTags.String != "null" {
			var tags []*dto2.SiteTagFullDTO
			if json.Unmarshal([]byte(siteTags.String), &tags) == nil {
				dto.SiteTags = tags
			}
		}

		// 解析 localAuthors
		if localAuthors.Valid && localAuthors.String != "" && localAuthors.String != "null" {
			var authors []*dto2.RankedLocalAuthor
			if json.Unmarshal([]byte(localAuthors.String), &authors) == nil {
				dto.LocalAuthors = authors
			}
		}

		// 解析 siteAuthors
		if siteAuthors.Valid && siteAuthors.String != "" && siteAuthors.String != "null" {
			var authors []*dto2.RankedSiteAuthor
			if json.Unmarshal([]byte(siteAuthors.String), &authors) == nil {
				dto.SiteAuthors = authors
			}
		}

		results = append(results, dto)
	}

	return results, total, nil
}

// namespaceCondition 构造 namespace 过滤片段。非空 namespace 返回 " AND rwt.namespace = ?" 与对应参数；
// 空 namespace 返回空串（不限制 namespace）。SQL 中 NULL=? 为 unknown 不命中，指定 namespace 时不匹配无 namespace 关联。
func namespaceCondition(namespace string) (string, []interface{}) {
	if namespace == "" {
		return "", nil
	}
	return " AND rwt.namespace = ?", []interface{}{namespace}
}

// buildWhereClause 根据搜索条件构建 WHERE 子句（别名 "t1"）
func buildWhereClause(conditions []*dto2.SearchCondition) (string, []interface{}) {
	return buildWhereClauseWithBaseline(conditions, "t1", "t1.deleted_at = 0")
}

// buildWhereClauseWithAlias 根据搜索条件构建 WHERE 子句（可配置表别名，软删基线=仅活行）
func buildWhereClauseWithAlias(conditions []*dto2.SearchCondition, alias string) (string, []interface{}) {
	return buildWhereClauseWithBaseline(conditions, alias, fmt.Sprintf("%s.deleted_at = 0", alias))
}

// buildWhereClauseWithBaseline 根据搜索条件构建 WHERE 子句（软删基线可配置：活行 "= 0" / 已删行 "> 0"）
func buildWhereClauseWithBaseline(conditions []*dto2.SearchCondition, alias string, baseline string) (string, []interface{}) {
	whereClauses := []string{baseline}
	var params []interface{}

	for _, cond := range conditions {
		if cond == nil {
			continue
		}

		switch cond.Type {
		case dto2.LocalTag:
			nsCond, nsParams := namespaceCondition(cond.Namespace)
			if cond.Operator == dto2.NotEqual {
				whereClauses = append(whereClauses,
					fmt.Sprintf(`NOT EXISTS(SELECT 1 FROM re_work_tag rwt
						LEFT JOIN site_tag st ON rwt.site_tag_id = st.id
						WHERE rwt.work_id = %s.id AND (rwt.local_tag_id = ? OR st.local_tag_id = ?)%s)`, alias, nsCond))
				params = append(params, cond.Value, cond.Value)
				params = append(params, nsParams...)
			} else {
				whereClauses = append(whereClauses,
					fmt.Sprintf(`EXISTS(SELECT 1 FROM re_work_tag rwt
						LEFT JOIN site_tag st ON rwt.site_tag_id = st.id
						WHERE rwt.work_id = %s.id AND (rwt.local_tag_id = ? OR st.local_tag_id = ?)%s)`, alias, nsCond))
				params = append(params, cond.Value, cond.Value)
				params = append(params, nsParams...)
			}

		case dto2.SiteTag:
			nsCond, nsParams := namespaceCondition(cond.Namespace)
			if cond.Operator == dto2.NotEqual {
				whereClauses = append(whereClauses,
					fmt.Sprintf("NOT EXISTS(SELECT 1 FROM re_work_tag rwt WHERE rwt.work_id = %s.id AND rwt.site_tag_id = ?%s)", alias, nsCond))
				params = append(params, cond.Value)
				params = append(params, nsParams...)
			} else {
				whereClauses = append(whereClauses,
					fmt.Sprintf("EXISTS(SELECT 1 FROM re_work_tag rwt WHERE rwt.work_id = %s.id AND rwt.site_tag_id = ?%s)", alias, nsCond))
				params = append(params, cond.Value)
				params = append(params, nsParams...)
			}

		case dto2.LocalAuthor:
			if cond.Operator == dto2.NotEqual {
				whereClauses = append(whereClauses,
					fmt.Sprintf(`NOT EXISTS(SELECT 1 FROM re_work_author rwa
						LEFT JOIN site_author sa ON rwa.site_author_id = sa.id
						WHERE rwa.work_id = %s.id AND (rwa.local_author_id = ? OR sa.local_author_id = ?))`, alias))
				params = append(params, cond.Value, cond.Value)
			} else {
				whereClauses = append(whereClauses,
					fmt.Sprintf(`EXISTS(SELECT 1 FROM re_work_author rwa
						LEFT JOIN site_author sa ON rwa.site_author_id = sa.id
						WHERE rwa.work_id = %s.id AND (rwa.local_author_id = ? OR sa.local_author_id = ?))`, alias))
				params = append(params, cond.Value, cond.Value)
			}

		case dto2.SiteAuthor:
			if cond.Operator == dto2.NotEqual {
				whereClauses = append(whereClauses,
					fmt.Sprintf("NOT EXISTS(SELECT 1 FROM re_work_author rwa WHERE rwa.work_id = %s.id AND rwa.site_author_id = ?)", alias))
				params = append(params, cond.Value)
			} else {
				whereClauses = append(whereClauses,
					fmt.Sprintf("EXISTS(SELECT 1 FROM re_work_author rwa WHERE rwa.work_id = %s.id AND rwa.site_author_id = ?)", alias))
				params = append(params, cond.Value)
			}

		case dto2.WorksSiteName:
			whereClauses = append(whereClauses, fmt.Sprintf("%s.site_work_name LIKE ?", alias))
			params = append(params, "%"+fmt.Sprintf("%v", cond.Value)+"%")

		case dto2.WorksNickname:
			whereClauses = append(whereClauses, fmt.Sprintf("%s.nick_name LIKE ?", alias))
			params = append(params, "%"+fmt.Sprintf("%v", cond.Value)+"%")

		case dto2.WorksUploadTime, dto2.WorksCreateTime, dto2.WorksDeleteTime:
			// 时间范围条件：按操作符生成（范围用 GreaterOrEqual/LessOrEqual，两端各传一条）
			column := "create_time"
			switch cond.Type {
			case dto2.WorksUploadTime:
				column = "site_upload_time"
			case dto2.WorksDeleteTime:
				column = "deleted_at"
			}
			op := "="
			switch cond.Operator {
			case dto2.GreaterThan:
				op = ">"
			case dto2.GreaterOrEqual:
				op = ">="
			case dto2.LessThan:
				op = "<"
			case dto2.LessOrEqual:
				op = "<="
			case dto2.NotEqual:
				op = "<>"
			}
			whereClauses = append(whereClauses, fmt.Sprintf("%s.%s %s ?", alias, column, op))
			params = append(params, cond.Value)

		case dto2.WorksLastView:
			if cond.Operator == dto2.NotEqual {
				whereClauses = append(whereClauses, fmt.Sprintf("%s.last_view <> ?", alias))
			} else {
				whereClauses = append(whereClauses, fmt.Sprintf("%s.last_view = ?", alias))
			}
			params = append(params, cond.Value)

		case dto2.Media:
			exts := getMediaExts(cond.Value)
			if len(exts) > 0 {
				placeholders := make([]string, len(exts))
				for i, ext := range exts {
					placeholders[i] = "?"
					params = append(params, ext)
				}
				if cond.Operator == dto2.NotEqual {
					whereClauses = append(whereClauses, fmt.Sprintf("%s.filename_extension NOT IN (%s)", alias, strings.Join(placeholders, ",")))
				} else {
					whereClauses = append(whereClauses, fmt.Sprintf("%s.filename_extension IN (%s)", alias, strings.Join(placeholders, ",")))
				}
			}

		case dto2.Site:
			if cond.Operator == dto2.NotEqual {
				whereClauses = append(whereClauses, fmt.Sprintf("%s.site_id <> ?", alias))
			} else {
				whereClauses = append(whereClauses, fmt.Sprintf("%s.site_id = ?", alias))
			}
			params = append(params, cond.Value)

		case dto2.WorkSet:
			// 「不在作品集 X 中」：X 的活性经 JOIN work_set 判定——已软删作品集的关联行虽保留（复原即回挂），
			// 但其成员不应再被本条件排除
			whereClauses = append(whereClauses,
				fmt.Sprintf(`NOT EXISTS(SELECT 1 FROM re_work_work_set rwws
					JOIN work_set ws ON rwws.work_set_id = ws.id AND ws.deleted_at = 0
					WHERE rwws.work_id = %s.id AND rwws.work_set_id = ?)`, alias))
			params = append(params, cond.Value)
		}
	}

	return "WHERE " + strings.Join(whereClauses, " AND "), params
}

// getMediaExts 获取媒体类型对应的扩展名列表
func getMediaExts(value interface{}) []string {
	mediaType, ok := value.(float64)
	if !ok {
		return nil
	}

	switch dto2.MediaType(mediaType) {
	case dto2.MediaTypePicture:
		return dto2.MediaExtMapping[dto2.MediaTypePicture]
	case dto2.MediaTypeVideo:
		return dto2.MediaExtMapping[dto2.MediaTypeVideo]
	case dto2.MediaTypeDocument:
		return dto2.MediaExtMapping[dto2.MediaTypeDocument]
	case dto2.MediaTypeAudio:
		return dto2.MediaExtMapping[dto2.MediaTypeAudio]
	}
	return nil
}

// QueryWorkSetPageByConditions 根据搜索条件查询作品集分页（EXISTS 子查询关联 work）
func (r *SearchRepository) QueryWorkSetPageByConditions(ctx context.Context, page, pageSize int, conditions []*dto2.SearchCondition) ([]*entity2.WorkSet, int64, error) {
	// 恒基线：作品集软删行不入正常列表（原生 SQL 不受 GORM 软删 scope 保护）
	clauses := []string{"work_set.deleted_at = 0"}
	var params []interface{}
	if len(conditions) > 0 {
		workWhere, workParams := buildWhereClauseWithAlias(conditions, "w")
		clauses = append(clauses, fmt.Sprintf(
			"EXISTS (SELECT 1 FROM work w INNER JOIN re_work_work_set rws ON w.id = rws.work_id WHERE rws.work_set_id = work_set.id AND %s)",
			strings.TrimPrefix(workWhere, "WHERE "),
		))
		params = workParams
	}
	whereClause := "WHERE " + strings.Join(clauses, " AND ")

	// 计算总数
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM work_set %s", whereClause)
	if err := r.db.WithContext(ctx).Raw(countQuery, params...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	querySQL := "SELECT work_set.id, work_set.create_time, work_set.update_time, " +
		"work_set.site_id, work_set.site_work_set_id, work_set.site_work_set_name, " +
		"work_set.site_author_id, work_set.site_work_set_description, " +
		"work_set.site_upload_time, work_set.site_update_time, " +
		"work_set.nick_name, work_set.last_view " +
		"FROM work_set " +
		whereClause + " " +
		"ORDER BY work_set.update_time DESC " +
		fmt.Sprintf("LIMIT %d OFFSET %d", pageSize, offset)
	rows, err := r.db.WithContext(ctx).Raw(querySQL, params...).Rows()
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []*entity2.WorkSet
	for rows.Next() {
		ws := entity2.NewWorkSet()
		if err := rows.Scan(
			&ws.ID, &ws.CreateTime, &ws.UpdateTime,
			&ws.SiteID, &ws.SiteWorkSetID, &ws.SiteWorkSetName,
			&ws.SiteAuthorID, &ws.SiteWorkSetDescription,
			&ws.SiteUploadTime, &ws.SiteUpdateTime,
			&ws.NickName, &ws.LastView,
		); err != nil {
			return nil, 0, err
		}
		results = append(results, ws)
	}

	return results, total, nil
}

// buildRecycleStoreWhere 构建回收站文件条目查询的 WHERE 子句（列表与 TTL 圈定共用同一构建函数防谓词漂移）。
// 基线两条：ps 已软删；挂载链不指向已软删作品（该形态聚合进回收站作品条目，提前清会破坏作品复原能力）。
// 挂载链断（resource_store/resource/work 行不存在）与 work 存活的软删行均纳入——离链孤儿自愈落入文件条目
func buildRecycleStoreWhere(query *dto2.RecycleStorePageQuery) (string, []interface{}) {
	clauses := []string{
		"ps.deleted_at > 0",
		`NOT EXISTS (SELECT 1 FROM resource_store rs
			JOIN resource r ON rs.resource_id = r.id
			JOIN work w ON r.work_id = w.id
			WHERE rs.store_id = ps.id AND w.deleted_at > 0)`,
	}
	var params []interface{}
	if query != nil {
		if query.FileName != "" {
			clauses = append(clauses, "ps.file_name LIKE ?")
			params = append(params, "%"+query.FileName+"%")
		}
		if query.FilePath != "" {
			clauses = append(clauses, "ps.file_path LIKE ?")
			params = append(params, "%"+query.FilePath+"%")
		}
		if query.MediaType != nil {
			if exts := dto2.MediaExtMapping[dto2.MediaType(*query.MediaType)]; len(exts) > 0 {
				placeholders := make([]string, len(exts))
				for i, ext := range exts {
					placeholders[i] = "?"
					params = append(params, ext)
				}
				clauses = append(clauses, fmt.Sprintf("ps.filename_extension IN (%s)", strings.Join(placeholders, ",")))
			}
		}
		if query.HasBackup != nil {
			if *query.HasBackup {
				clauses = append(clauses, "ps.backup_id > 0")
			} else {
				clauses = append(clauses, "ps.backup_id = 0")
			}
		}
		if query.DeleteTimeFrom > 0 {
			clauses = append(clauses, "ps.deleted_at >= ?")
			params = append(params, query.DeleteTimeFrom)
		}
		if query.DeleteTimeTo > 0 {
			clauses = append(clauses, "ps.deleted_at <= ?")
			params = append(params, query.DeleteTimeTo)
		}
		if query.WorkName != "" {
			clauses = append(clauses, `EXISTS (SELECT 1 FROM resource_store rs2
				JOIN resource r2 ON rs2.resource_id = r2.id
				JOIN work w2 ON r2.work_id = w2.id
				WHERE rs2.store_id = ps.id AND w2.deleted_at = 0 AND w2.site_work_name LIKE ?)`)
			params = append(params, "%"+query.WorkName+"%")
		}
	}
	return "WHERE " + strings.Join(clauses, " AND "), params
}

// QueryRecycleStorePage 查询回收站文件条目分页（persistent_store 已删行，非「作品已删」聚合形态）。
// 行本体单查投影；作品上下文（挂载活作品的名称/站点）经二段批查组装——分页行收集 ID 一次 JOIN，
// 消除逐行查询。CanRestore = backup_id>0 且挂载链存在活作品（防御式独立判定，不依赖主谓词排除）
func (r *SearchRepository) QueryRecycleStorePage(ctx context.Context, page, pageSize int, query *dto2.RecycleStorePageQuery) ([]*dto2.RecycleStoreDTO, int64, error) {
	whereClause, params := buildRecycleStoreWhere(query)

	orderBy := "ps.deleted_at DESC, ps.id DESC"
	if query != nil && query.SortOrder == "asc" {
		orderBy = "ps.deleted_at ASC, ps.id DESC"
	}

	// 计数
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM persistent_store ps %s", whereClause)
	if err := r.db.WithContext(ctx).Raw(countQuery, params...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	querySQL := fmt.Sprintf(`
		SELECT ps.id, ps.file_name, ps.file_path, ps.filename_extension, ps.deleted_at, ps.backup_id
		FROM persistent_store ps
		%s
		ORDER BY %s
		LIMIT %d OFFSET %d
	`, whereClause, orderBy, pageSize, offset)

	rows, err := r.db.WithContext(ctx).Raw(querySQL, params...).Rows()
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	results := make([]*dto2.RecycleStoreDTO, 0, pageSize)
	for rows.Next() {
		item := &dto2.RecycleStoreDTO{}
		var fileName, filePath, ext sql.NullString
		var backupId int64
		if err := rows.Scan(
			&item.ID,
			&fileName,
			&filePath,
			&ext,
			&item.DeleteTime,
			&backupId,
		); err != nil {
			return nil, 0, err
		}
		item.FileName = nullStr(fileName)
		item.FilePath = nullStr(filePath)
		item.FilenameExtension = nullStr(ext)
		item.HasBackup = backupId > 0
		results = append(results, item)
	}

	// 二段批查：分页行的挂载上下文（resource_store→resource→work 活作品 + 站点名），回填作品字段与可复原性
	mounts := r.queryStoreMountContext(ctx, collectStoreIds(results))
	for _, item := range results {
		if mount, ok := mounts[item.ID]; ok {
			workId := mount.workId
			item.WorkId = &workId
			item.WorkName = mount.workName
			item.SiteName = mount.siteName
			item.CanRestore = item.HasBackup
		}
	}
	return results, total, nil
}

// ListRecycleStoreIdsDeletedBefore 圈定删除时间早于 expireBefore（毫秒时间戳）的文件条目 ID（TTL 清理）。
// 与列表查询共用 buildRecycleStoreWhere——「作品已删」聚合行不被圈定（保护作品条目的复原能力）
func (r *SearchRepository) ListRecycleStoreIdsDeletedBefore(ctx context.Context, expireBefore int64) ([]int64, error) {
	whereClause, params := buildRecycleStoreWhere(nil)
	querySQL := fmt.Sprintf("SELECT ps.id FROM persistent_store ps %s AND ps.deleted_at < ?", whereClause)
	var ids []int64
	err := r.db.WithContext(ctx).Raw(querySQL, append(params, expireBefore)...).Pluck("id", &ids).Error
	return ids, err
}

// storeMountContext 一行挂载上下文（resource_store→resource→work 链 + 站点名；仅活作品挂载）
type storeMountContext struct {
	storeId  int64
	workId   int64
	workName string
	siteName string
}

// queryStoreMountContext 批查 store 行的挂载上下文（仅登记活作品挂载；「作品已删」挂载被主谓词排除，
// 此处再过滤为防御）。同 store 多挂载时取首个活挂载（展示用途，挂载唯一性无契约）
func (r *SearchRepository) queryStoreMountContext(ctx context.Context, storeIds []int64) map[int64]*storeMountContext {
	result := make(map[int64]*storeMountContext, len(storeIds))
	if len(storeIds) == 0 {
		return result
	}
	placeholders := make([]string, len(storeIds))
	params := make([]interface{}, len(storeIds))
	for i, id := range storeIds {
		placeholders[i] = "?"
		params[i] = id
	}
	querySQL := fmt.Sprintf(`
		SELECT rs.store_id, w.id, w.site_work_name, COALESCE(s.site_name, '')
		FROM resource_store rs
		JOIN resource r ON rs.resource_id = r.id
		JOIN work w ON r.work_id = w.id AND w.deleted_at = 0
		LEFT JOIN site s ON w.site_id = s.id
		WHERE rs.store_id IN (%s)
	`, strings.Join(placeholders, ","))
	rows, err := r.db.WithContext(ctx).Raw(querySQL, params...).Rows()
	if err != nil {
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var mount storeMountContext
		var workName sql.NullString
		if err := rows.Scan(&mount.storeId, &mount.workId, &workName, &mount.siteName); err != nil {
			return result
		}
		mount.workName = nullStr(workName)
		if _, exists := result[mount.storeId]; !exists {
			result[mount.storeId] = &mount
		}
	}
	return result
}

// GetRecycleStoreMount 查询单个 store 行的挂载身份（首条关联行的 resource_id/store_type/store_seq
// 与所属作品活性）。无关联行返回 ResourceId=0。回收站复原置换链用
func (r *SearchRepository) GetRecycleStoreMount(ctx context.Context, storeId int64) (*dto2.StoreMountDTO, error) {
	querySQL := `
		SELECT rs.resource_id, rs.store_type, rs.store_seq,
		       EXISTS(SELECT 1 FROM resource r2 JOIN work w2 ON r2.work_id = w2.id AND w2.deleted_at = 0
		              WHERE r2.id = rs.resource_id)
		FROM resource_store rs
		WHERE rs.store_id = ?
		LIMIT 1`
	row := r.db.WithContext(ctx).Raw(querySQL, storeId).Row()
	mount := &dto2.StoreMountDTO{}
	if err := row.Scan(&mount.ResourceId, &mount.Role, &mount.Seq, &mount.WorkAlive); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows) {
			return &dto2.StoreMountDTO{}, nil
		}
		return nil, err
	}
	return mount, nil
}

// GetAliveStoreIdByKey 查挂载键 (resource_id, store_type, store_seq) 下的活行 store ID（无则 0）。
// 挂载不变量：每键活行至多一条。复原置换链圈定当前代用
func (r *SearchRepository) GetAliveStoreIdByKey(ctx context.Context, resourceId int64, storeType string, storeSeq int) (int64, error) {
	querySQL := `
		SELECT rs.store_id
		FROM resource_store rs
		JOIN persistent_store ps ON rs.store_id = ps.id AND ps.deleted_at = 0
		WHERE rs.resource_id = ? AND rs.store_type = ? AND rs.store_seq = ?
		LIMIT 1`
	var storeId int64
	err := r.db.WithContext(ctx).Raw(querySQL, resourceId, storeType, storeSeq).Scan(&storeId).Error
	return storeId, err
}

// collectStoreIds 从分页结果收集 store ID（二段批查入参）
func collectStoreIds(items []*dto2.RecycleStoreDTO) []int64 {
	ids := make([]int64, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

// nullStr sql.NullString 取值（NULL 归空串）
func nullStr(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}

// buildRecycleWorkSetWhere 构建回收站作品集条目查询的 WHERE 子句。基线 = work_set 已软删行；
// 活成员数子查询按 work 活行计数（已删成员不计）
func buildRecycleWorkSetWhere(query *dto2.RecycleWorkSetPageQuery) (string, []interface{}) {
	clauses := []string{"ws.deleted_at > 0"}
	var params []interface{}
	if query != nil {
		if query.Name != "" {
			clauses = append(clauses, "(ws.site_work_set_name LIKE ? OR ws.nick_name LIKE ?)")
			params = append(params, "%"+query.Name+"%", "%"+query.Name+"%")
		}
		if query.SiteId != nil {
			clauses = append(clauses, "ws.site_id = ?")
			params = append(params, *query.SiteId)
		}
		if query.DeleteTimeFrom > 0 {
			clauses = append(clauses, "ws.deleted_at >= ?")
			params = append(params, query.DeleteTimeFrom)
		}
		if query.DeleteTimeTo > 0 {
			clauses = append(clauses, "ws.deleted_at <= ?")
			params = append(params, query.DeleteTimeTo)
		}
	}
	return "WHERE " + strings.Join(clauses, " AND "), params
}

// QueryRecycleWorkSetPage 查询回收站作品集条目分页（work_set 已删行；作品集域平铺条件体系）
func (r *SearchRepository) QueryRecycleWorkSetPage(ctx context.Context, page, pageSize int, query *dto2.RecycleWorkSetPageQuery) ([]*dto2.RecycleWorkSetDTO, int64, error) {
	whereClause, params := buildRecycleWorkSetWhere(query)

	orderBy := "ws.deleted_at DESC, ws.id DESC"
	if query != nil && query.SortOrder == "asc" {
		orderBy = "ws.deleted_at ASC, ws.id DESC"
	}

	// 计数
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM work_set ws %s", whereClause)
	if err := r.db.WithContext(ctx).Raw(countQuery, params...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	querySQL := fmt.Sprintf(`
		SELECT ws.id, ws.site_id, ws.site_work_set_id, ws.site_work_set_name, ws.nick_name,
			ws.create_time, ws.deleted_at,
			COALESCE(s.site_name, '') AS site_name,
			(SELECT COUNT(*) FROM re_work_work_set rwws
				JOIN work w ON rwws.work_id = w.id AND w.deleted_at = 0
				WHERE rwws.work_set_id = ws.id) AS alive_member_count
		FROM work_set ws
		LEFT JOIN site s ON ws.site_id = s.id
		%s
		ORDER BY %s
		LIMIT %d OFFSET %d
	`, whereClause, orderBy, pageSize, offset)

	rows, err := r.db.WithContext(ctx).Raw(querySQL, params...).Rows()
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	results := make([]*dto2.RecycleWorkSetDTO, 0, pageSize)
	for rows.Next() {
		item := &dto2.RecycleWorkSetDTO{}
		var siteId sql.NullInt64
		var siteWorkSetId, name, nickName sql.NullString
		if err := rows.Scan(
			&item.ID,
			&siteId,
			&siteWorkSetId,
			&name,
			&nickName,
			&item.CreateTime,
			&item.DeleteTime,
			&item.SiteName,
			&item.AliveMemberCount,
		); err != nil {
			return nil, 0, err
		}
		if siteId.Valid {
			v := siteId.Int64
			item.SiteID = &v
		}
		if siteWorkSetId.Valid {
			v := siteWorkSetId.String
			item.SiteWorkSetID = &v
		}
		item.Name = nullStr(name)
		item.NickName = nullStr(nickName)
		results = append(results, item)
	}
	return results, total, nil
}

// containsType 检查类型切片是否包含指定类型
func containsType(types []dto2.SearchType, target dto2.SearchType) bool {
	for _, t := range types {
		if t == target {
			return true
		}
	}
	return false
}

// QueryRecycleWorkPage 查询回收站作品分页（work 已删行，条件体系复用 buildWhereClauseWithBaseline，
// 基线为 deleted_at > 0；排序支持 deleted_at/create_time；投影为回收站列表精简列，站点名 LEFT JOIN、
// 作者名子查询聚合——本地作者名优先，无本地关联回退站点作者名，顿号拼接）
func (r *SearchRepository) QueryRecycleWorkPage(ctx context.Context, page, pageSize int, conditions []*dto2.SearchCondition, sortField string, sortDesc bool) ([]*dto2.RecycleWorkDTO, int64, error) {
	whereClause, params := buildWhereClauseWithBaseline(conditions, "t1", "t1.deleted_at > 0")

	orderBy := "t1.deleted_at DESC"
	if sortField == "create_time" {
		orderBy = fmt.Sprintf("t1.create_time %s, t1.id DESC", map[bool]string{true: "DESC", false: "ASC"}[sortDesc])
	} else if !sortDesc {
		orderBy = "t1.deleted_at ASC, t1.id DESC"
	}

	// 计算总数
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM work t1 %s", whereClause)
	if err := r.db.WithContext(ctx).Raw(countQuery, params...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	query := fmt.Sprintf(`
		SELECT t1.id, t1.site_id, t1.site_work_id, t1.site_work_name, t1.create_time, t1.deleted_at,
			COALESCE(s.site_name, '') AS site_name,
			(SELECT GROUP_CONCAT(COALESCE(la.author_name, sa.author_name), '、')
				FROM re_work_author rwa
				LEFT JOIN local_author la ON rwa.local_author_id = la.id
				LEFT JOIN site_author sa ON rwa.site_author_id = sa.id
				WHERE t1.id = rwa.work_id) AS author_names,
			COALESCE(
				(SELECT ps.file_path FROM resource r
					JOIN resource_store rs ON rs.resource_id = r.id AND rs.store_type = 'thumbnail'
					JOIN persistent_store ps ON rs.store_id = ps.id
					WHERE t1.id = r.work_id LIMIT 1),
				(SELECT ps.file_path FROM resource r
					JOIN resource_store rs ON rs.resource_id = r.id AND rs.store_type = 'image'
					JOIN persistent_store ps ON rs.store_id = ps.id
					WHERE t1.id = r.work_id AND r.resource_type = 'image' LIMIT 1)
			) AS preview_path
		FROM work t1
		LEFT JOIN site s ON t1.site_id = s.id
		%s
		ORDER BY %s
		LIMIT %d OFFSET %d
	`, whereClause, orderBy, pageSize, offset)

	rows, err := r.db.WithContext(ctx).Raw(query, params...).Rows()
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []*dto2.RecycleWorkDTO
	for rows.Next() {
		var item dto2.RecycleWorkDTO
		var siteId sql.NullInt64
		var siteWorkId, workName, authorNames, previewPath sql.NullString
		if err := rows.Scan(
			&item.ID,
			&siteId,
			&siteWorkId,
			&workName,
			&item.CreateTime,
			&item.DeleteTime,
			&item.SiteName,
			&authorNames,
			&previewPath,
		); err != nil {
			return nil, 0, err
		}
		if siteId.Valid {
			v := siteId.Int64
			item.SiteID = &v
		}
		if siteWorkId.Valid {
			v := siteWorkId.String
			item.SiteWorkID = &v
		}
		if workName.Valid {
			v := workName.String
			item.WorkName = &v
		}
		if authorNames.Valid {
			item.AuthorNames = authorNames.String
		}
		if previewPath.Valid {
			item.PreviewPath = previewPath.String
		}
		results = append(results, &item)
	}
	return results, total, nil
}
