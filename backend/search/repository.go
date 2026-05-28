package search

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	dto2 "github.com/library-squirrel/backend/base/model/dto"
	entity2 "github.com/library-squirrel/backend/base/model/entity"
	sdkdto "github.com/lvfeng-z/library-squirrel-plugin-sdk/dto"

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
func (r *SearchRepository) QuerySearchConditionPage(ctx context.Context, page, pageSize int, keyword string, types []sdkdto.SearchType) ([]*sdkdto.SelectItem, int64, error) {
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
	includeLocalTag := len(types) == 0 || containsType(types, sdkdto.SearchTypeLocalTag)
	includeSiteTag := len(types) == 0 || containsType(types, sdkdto.SearchTypeSiteTag)
	includeLocalAuthor := len(types) == 0 || containsType(types, sdkdto.SearchTypeLocalAuthor)
	includeSiteAuthor := len(types) == 0 || containsType(types, sdkdto.SearchTypeSiteAuthor)

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
				 'site', JSON_OBJECT('id', COALESCE(t3.id, 0), 'siteName', COALESCE(t3.site_name, ''), 'siteDescription', COALESCE(t3.site_description, ''))
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
		return []*sdkdto.SelectItem{}, 0, nil
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

	var results []*sdkdto.SelectItem
	for rows.Next() {
		var item sdkdto.SelectItem
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
					switch sdkdto.SearchType(typeVal) {
					case sdkdto.SearchTypeLocalTag:
						subLabels = append(subLabels, "tag", "local")
					case sdkdto.SearchTypeSiteTag:
						subLabels = append(subLabels, "tag")
						if siteMap, ok := extraMap["site"].(map[string]interface{}); ok {
							if siteName, ok := siteMap["siteName"].(string); ok && siteName != "" {
								subLabels = append(subLabels, siteName)
							} else {
								subLabels = append(subLabels, "?")
							}
						}
					case sdkdto.SearchTypeLocalAuthor:
						subLabels = append(subLabels, "author", "local")
					case sdkdto.SearchTypeSiteAuthor:
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
func (r *SearchRepository) QueryWorkPage(ctx context.Context, page, pageSize int, conditions []*sdkdto.SearchCondition) ([]*sdkdto.WorkFullDTO, int64, error) {
	// 构建 WHERE 子句
	whereClause, params := buildWhereClause(conditions)

	// 计算总数
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(DISTINCT t1.id) FROM work t1 %s", whereClause)
	if err := r.db.WithContext(ctx).Raw(countQuery, params...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	// 分页查询
	offset := (page - 1) * pageSize
	query := fmt.Sprintf(`
		SELECT t1.id, t1.create_time, t1.update_time, t1.site_id, t1.site_work_id, t1.site_work_name,
				t1.site_author_id, t1.site_work_description, t1.site_upload_time, t1.site_update_time,
				t1.nick_name, t1.local_author_id, t1.last_view,
			CASE WHEN t2.id IS NOT NULL THEN
				JSON_OBJECT('id', t2.id, 'workId', t2.work_id, 'taskId', t2.task_id, 'state', t2.state, 'filePath', t2.file_path, 'fileName', t2.file_name,
					'filenameExtension', t2.filename_extension, 'suggestName', t2.suggest_name, 'workdir', t2.workdir, 'resourceComplete', t2.resource_complete)
			END AS resource,
			(SELECT JSON_GROUP_ARRAY(JSON_OBJECT(
				'id', rt1.id, 'workId', rt1.work_id, 'taskId', rt1.task_id, 'state', rt1.state, 'filePath', rt1.file_path, 'fileName', rt1.file_name, 'filenameExtension',
				rt1.filename_extension, 'suggestName', rt1.suggest_name, 'workdir', rt1.workdir, 'resourceComplete', rt1.resource_complete))
			FROM resource rt1
			WHERE t1.id = rt1.work_id AND rt1.state = 0) AS inactiveResource,
			(SELECT JSON_GROUP_ARRAY(JSON_OBJECT('id', rt2.id, 'localTagName', rt2.local_tag_name, 'baseLocalTagId', rt2.base_local_tag_id, 'lastUse', rt2.last_use))
			FROM re_work_tag rt1
			INNER JOIN local_tag rt2 ON rt1.local_tag_id = rt2.id
			WHERE t1.id = rt1.work_id) AS localTags,
			(SELECT JSON_GROUP_ARRAY(JSON_OBJECT(
				'id', rt2.id, 'siteId', rt2.site_id, 'siteTagId', rt2.site_tag_id, 'siteTagName', rt2.site_tag_name, 'baseSiteTagId', rt2.base_site_tag_id,
				'description', rt2.description, 'localTagId', rt2.local_tag_id, 'lastUse', rt2.last_use))
			FROM re_work_tag rt1
			INNER JOIN site_tag rt2 ON rt1.site_tag_id = rt2.id
			WHERE t1.id = rt1.work_id) AS siteTags,
			(SELECT JSON_GROUP_ARRAY(JSON_OBJECT('id', rt2.id, 'authorName', rt2.author_name, 'lastUse', rt2.last_use, 'authorRank', rt1.author_rank))
			FROM re_work_author rt1
			INNER JOIN local_author rt2 ON rt1.local_author_id = rt2.id
			WHERE t1.id = rt1.work_id) AS localAuthors,
			(SELECT JSON_GROUP_ARRAY(JSON_OBJECT(
				'id', rt2.id, 'siteId', rt2.site_id, 'siteAuthorId', rt2.site_author_id, 'authorName', rt2.author_name, 'siteAuthorNameBefore',
				rt2.site_author_name_before, 'introduce', rt2.introduce, 'localAuthorId', rt2.local_author_id, 'lastUse', rt2.last_use, 'authorRank', rt1.author_rank))
			FROM re_work_author rt1
			INNER JOIN site_author rt2 ON rt1.site_author_id = rt2.id
			WHERE t1.id = rt1.work_id) AS siteAuthors,
			(SELECT JSON_GROUP_ARRAY(JSON_OBJECT(
				'id', rt2.id, 'siteId', rt2.site_id, 'siteWorkSetId', rt2.site_work_set_id, 'siteWorkSetName', rt2.site_work_set_name, 'siteAuthorId', rt2.site_author_id,
				'siteUploadTime', rt2.site_upload_time, 'siteUpdateTime', rt2.site_update_time, 'nickName', rt2.nick_name, 'lastView', rt2.last_view))
			FROM re_work_work_set rt1
			INNER JOIN work_set rt2 ON rt1.work_set_id = rt2.id
			WHERE t1.id = rt1.work_id) AS workSets
		FROM work t1
		LEFT JOIN resource t2 ON t1.id = t2.work_id AND t2.state = 1
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

	var results []*sdkdto.WorkFullDTO
	for rows.Next() {
		work := entity2.NewWork()
		var resource, inactiveResource, localTags, siteTags, localAuthors, siteAuthors, workSets sql.NullString

		if err := rows.Scan(
			&work.ID, &work.CreateTime, &work.UpdateTime, &work.SiteID, &work.SiteWorkID, &work.SiteWorkName,
			&work.SiteAuthorID, &work.SiteWorkDescription, &work.SiteUploadTime, &work.SiteUpdateTime,
			&work.NickName, &work.LocalAuthorID, &work.LastView,
			&resource, &inactiveResource, &localTags, &siteTags, &localAuthors, &siteAuthors, &workSets,
		); err != nil {
			return nil, 0, err
		}

		dto := dto2.NewWorkFullDTO(work)

		// 解析 resource JSON
		if resource.Valid && resource.String != "" {
			var res sdkdto.ResourceDTO
			if json.Unmarshal([]byte(resource.String), &res) == nil {
				dto.Resources = append(dto.Resources, &res)
			}
		}

		// 解析 inactiveResource JSON 数组
		if inactiveResource.Valid && inactiveResource.String != "" {
			var resList []sdkdto.ResourceDTO
			if json.Unmarshal([]byte(inactiveResource.String), &resList) == nil {
				for i := range resList {
					dto.Resources = append(dto.Resources, &resList[i])
				}
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
			var tags []*sdkdto.SiteTagFullDTO
			if json.Unmarshal([]byte(siteTags.String), &tags) == nil {
				dto.SiteTags = tags
			}
		}

		// 解析 localAuthors
		if localAuthors.Valid && localAuthors.String != "" && localAuthors.String != "null" {
			var authors []*sdkdto.LocalAuthorDTO
			if json.Unmarshal([]byte(localAuthors.String), &authors) == nil {
				dto.LocalAuthors = authors
			}
		}

		// 解析 siteAuthors
		if siteAuthors.Valid && siteAuthors.String != "" && siteAuthors.String != "null" {
			var authors []*sdkdto.SiteAuthorFullDTO
			if json.Unmarshal([]byte(siteAuthors.String), &authors) == nil {
				dto.SiteAuthors = authors
			}
		}

		results = append(results, dto)
	}

	return results, total, nil
}

// buildWhereClause 根据搜索条件构建 WHERE 子句（别名 "t1"）
func buildWhereClause(conditions []*sdkdto.SearchCondition) (string, []interface{}) {
	return buildWhereClauseWithAlias(conditions, "t1")
}

// buildWhereClauseWithAlias 根据搜索条件构建 WHERE 子句（可配置表别名）
func buildWhereClauseWithAlias(conditions []*sdkdto.SearchCondition, alias string) (string, []interface{}) {
	if len(conditions) == 0 {
		return "", nil
	}

	var whereClauses []string
	var params []interface{}

	for _, cond := range conditions {
		if cond == nil {
			continue
		}

		switch cond.Type {
		case sdkdto.SearchTypeLocalTag:
			if cond.Operator == sdkdto.OperatorNotEqual {
				whereClauses = append(whereClauses,
					fmt.Sprintf(`NOT EXISTS(SELECT 1 FROM re_work_tag rwt
						LEFT JOIN site_tag st ON rwt.site_tag_id = st.id
						WHERE rwt.work_id = %s.id AND (rwt.local_tag_id = ? OR st.local_tag_id = ?))`, alias))
				params = append(params, cond.Value, cond.Value)
			} else {
				whereClauses = append(whereClauses,
					fmt.Sprintf(`EXISTS(SELECT 1 FROM re_work_tag rwt
						LEFT JOIN site_tag st ON rwt.site_tag_id = st.id
						WHERE rwt.work_id = %s.id AND (rwt.local_tag_id = ? OR st.local_tag_id = ?))`, alias))
				params = append(params, cond.Value, cond.Value)
			}

		case sdkdto.SearchTypeSiteTag:
			if cond.Operator == sdkdto.OperatorNotEqual {
				whereClauses = append(whereClauses,
					fmt.Sprintf("NOT EXISTS(SELECT 1 FROM re_work_tag rwt WHERE rwt.work_id = %s.id AND rwt.site_tag_id = ?)", alias))
				params = append(params, cond.Value)
			} else {
				whereClauses = append(whereClauses,
					fmt.Sprintf("EXISTS(SELECT 1 FROM re_work_tag rwt WHERE rwt.work_id = %s.id AND rwt.site_tag_id = ?)", alias))
				params = append(params, cond.Value)
			}

		case sdkdto.SearchTypeLocalAuthor:
			if cond.Operator == sdkdto.OperatorNotEqual {
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

		case sdkdto.SearchTypeSiteAuthor:
			if cond.Operator == sdkdto.OperatorNotEqual {
				whereClauses = append(whereClauses,
					fmt.Sprintf("NOT EXISTS(SELECT 1 FROM re_work_author rwa WHERE rwa.work_id = %s.id AND rwa.site_author_id = ?)", alias))
				params = append(params, cond.Value)
			} else {
				whereClauses = append(whereClauses,
					fmt.Sprintf("EXISTS(SELECT 1 FROM re_work_author rwa WHERE rwa.work_id = %s.id AND rwa.site_author_id = ?)", alias))
				params = append(params, cond.Value)
			}

		case sdkdto.SearchTypeWorksSiteName:
			whereClauses = append(whereClauses, fmt.Sprintf("%s.site_work_name LIKE ?", alias))
			params = append(params, "%"+fmt.Sprintf("%v", cond.Value)+"%")

		case sdkdto.SearchTypeWorksNickname:
			whereClauses = append(whereClauses, fmt.Sprintf("%s.nick_name LIKE ?", alias))
			params = append(params, "%"+fmt.Sprintf("%v", cond.Value)+"%")

		case sdkdto.SearchTypeWorksUploadTime:
			if cond.Operator == sdkdto.OperatorNotEqual {
				whereClauses = append(whereClauses, fmt.Sprintf("%s.site_upload_time <> ?", alias))
			} else {
				whereClauses = append(whereClauses, fmt.Sprintf("%s.site_upload_time = ?", alias))
			}
			params = append(params, cond.Value)

		case sdkdto.SearchTypeWorksLastView:
			if cond.Operator == sdkdto.OperatorNotEqual {
				whereClauses = append(whereClauses, fmt.Sprintf("%s.last_view <> ?", alias))
			} else {
				whereClauses = append(whereClauses, fmt.Sprintf("%s.last_view = ?", alias))
			}
			params = append(params, cond.Value)

		case sdkdto.SearchTypeMediaType:
			exts := getMediaExts(cond.Value)
			if len(exts) > 0 {
				placeholders := make([]string, len(exts))
				for i, ext := range exts {
					placeholders[i] = "?"
					params = append(params, ext)
				}
				if cond.Operator == sdkdto.OperatorNotEqual {
					whereClauses = append(whereClauses, fmt.Sprintf("%s.filename_extension NOT IN (%s)", alias, strings.Join(placeholders, ",")))
				} else {
					whereClauses = append(whereClauses, fmt.Sprintf("%s.filename_extension IN (%s)", alias, strings.Join(placeholders, ",")))
				}
			}

		case sdkdto.SearchTypeSite:
			if cond.Operator == sdkdto.OperatorNotEqual {
				whereClauses = append(whereClauses, fmt.Sprintf("%s.site_id <> ?", alias))
			} else {
				whereClauses = append(whereClauses, fmt.Sprintf("%s.site_id = ?", alias))
			}
			params = append(params, cond.Value)

		case sdkdto.SearchTypeWorkSet:
			whereClauses = append(whereClauses,
				fmt.Sprintf("NOT EXISTS(SELECT 1 FROM re_work_work_set rwws WHERE rwws.work_id = %s.id AND rwws.work_set_id = ?)", alias))
			params = append(params, cond.Value)
		}
	}

	if len(whereClauses) == 0 {
		return "", nil
	}

	return "WHERE " + strings.Join(whereClauses, " AND "), params
}

// getMediaExts 获取媒体类型对应的扩展名列表
func getMediaExts(value interface{}) []string {
	mediaType, ok := value.(float64)
	if !ok {
		return nil
	}

	switch sdkdto.MediaType(mediaType) {
	case sdkdto.MediaTypePicture:
		return sdkdto.MediaExtMapping[sdkdto.MediaTypePicture]
	case sdkdto.MediaTypeVideo:
		return sdkdto.MediaExtMapping[sdkdto.MediaTypeVideo]
	case sdkdto.MediaTypeDocument:
		return sdkdto.MediaExtMapping[sdkdto.MediaTypeDocument]
	case sdkdto.MediaTypeAudio:
		return sdkdto.MediaExtMapping[sdkdto.MediaTypeAudio]
	}
	return nil
}

// QueryWorkSetPageByConditions 根据搜索条件查询作品集分页（EXISTS 子查询关联 work）
func (r *SearchRepository) QueryWorkSetPageByConditions(ctx context.Context, page, pageSize int, conditions []*sdkdto.SearchCondition) ([]*entity2.WorkSet, int64, error) {
	var whereClause string
	var params []interface{}

	if len(conditions) > 0 {
		workWhere, workParams := buildWhereClauseWithAlias(conditions, "w")
		if workWhere != "" {
			innerConditions := strings.TrimPrefix(workWhere, "WHERE ")
			whereClause = fmt.Sprintf(
				"WHERE EXISTS (SELECT 1 FROM work w INNER JOIN re_work_work_set rws ON w.id = rws.work_id WHERE rws.work_set_id = work_set.id AND %s)",
				innerConditions,
			)
			params = workParams
		}
	}

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

// containsType 检查类型切片是否包含指定类型
func containsType(types []sdkdto.SearchType, target sdkdto.SearchType) bool {
	for _, t := range types {
		if t == target {
			return true
		}
	}
	return false
}
