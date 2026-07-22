package entity

import (
	"testing"
)

// mkStore 构造指定 store_type 的 ResourceStore 测试夹具
func mkStore(storeType string, storeID int64) *ResourceStore {
	s := NewResourceStore()
	s.StoreType = storeType
	s.StoreID = storeID
	return s
}

func TestResourceTypeRegistry_Coverage(t *testing.T) {
	// 5 个预定义类型全部注册
	expected := []string{ResourceTypeImage, ResourceTypeVideo, ResourceTypeArticle, ResourceTypeDocument, ResourceTypeUnknown}
	for _, rt := range expected {
		spec, ok := ResourceTypeRegistry[rt]
		if !ok {
			t.Fatalf("预定义类型 %s 未注册", rt)
		}
		if spec.ResourceType != rt {
			t.Fatalf("类型 %s 的 spec.ResourceType 不匹配: %s", rt, spec.ResourceType)
		}
		// 非 unknown 类型必有 PrimaryRoles 与 Roles
		if rt != ResourceTypeUnknown {
			if len(spec.PrimaryRoles) == 0 {
				t.Fatalf("类型 %s 缺 PrimaryRoles", rt)
			}
			if len(spec.Roles) == 0 {
				t.Fatalf("类型 %s 缺 Roles", rt)
			}
		}
	}
	// unknown 无约束
	if uk := ResourceTypeRegistry[ResourceTypeUnknown]; uk.Roles != nil || uk.PrimaryRoles != nil {
		t.Fatalf("unknown 应无 Roles/PrimaryRoles 约束, 实际 %+v", uk)
	}
}

func TestValidateResourceStructure(t *testing.T) {
	cases := []struct {
		name       string
		rt         string
		counts     map[string]int
		wantMiss   []string
		wantExcess []string
	}{
		{
			name:   "video 完整(videoTrack+audioTrack)",
			rt:     ResourceTypeVideo,
			counts: map[string]int{StoreTypeVideoTrack: 1, StoreTypeAudioTrack: 1},
		},
		{
			name:     "video 缺 audioTrack",
			rt:       ResourceTypeVideo,
			counts:   map[string]int{StoreTypeVideoTrack: 1},
			wantMiss: []string{StoreTypeAudioTrack},
		},
		{
			name:       "video merged 超量(2>Max1)",
			rt:         ResourceTypeVideo,
			counts:     map[string]int{StoreTypeVideoTrack: 1, StoreTypeAudioTrack: 1, StoreTypeMerged: 2},
			wantExcess: []string{StoreTypeMerged},
		},
		{
			name:   "image 完整(image+thumbnail)",
			rt:     ResourceTypeImage,
			counts: map[string]int{StoreTypeImage: 1, StoreTypeThumbnail: 1},
		},
		{
			name:   "article 内嵌图不限(N 个不报 excess)",
			rt:     ResourceTypeArticle,
			counts: map[string]int{StoreTypeDocument: 1, StoreTypeImage: 10},
		},
		{
			name:   "article 缺正文 document",
			rt:     ResourceTypeArticle,
			counts: map[string]int{StoreTypeImage: 3},
			wantMiss: []string{StoreTypeDocument},
		},
		{
			name:   "unknown 不校验",
			rt:     ResourceTypeUnknown,
			counts: map[string]int{StoreTypeImage: 1},
		},
		{
			name:   "空 resourceType 不校验",
			rt:     "",
			counts: map[string]int{StoreTypeImage: 1},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			miss, excess := ValidateResourceStructure(c.rt, c.counts)
			if !sameSet(miss, c.wantMiss) {
				t.Fatalf("missing 期望 %v, 实际 %v", c.wantMiss, miss)
			}
			if !sameSet(excess, c.wantExcess) {
				t.Fatalf("excess 期望 %v, 实际 %v", c.wantExcess, excess)
			}
		})
	}
}

func TestResolvePrimaryStore(t *testing.T) {
	// video 有 merged → merged
	videoSpec := LookupResourceTypeSpec(ResourceTypeVideo)
	got := ResolvePrimaryStore([]*ResourceStore{
		mkStore(StoreTypeVideoTrack, 1),
		mkStore(StoreTypeAudioTrack, 2),
		mkStore(StoreTypeMerged, 3),
	}, videoSpec)
	if got == nil || got.StoreType != StoreTypeMerged {
		t.Fatalf("video 有 merged 期望取 merged, 实际 %+v", got)
	}
	// video 无 merged → videoTrack
	got = ResolvePrimaryStore([]*ResourceStore{
		mkStore(StoreTypeVideoTrack, 1),
		mkStore(StoreTypeAudioTrack, 2),
	}, videoSpec)
	if got == nil || got.StoreType != StoreTypeVideoTrack {
		t.Fatalf("video 无 merged 期望取 videoTrack, 实际 %+v", got)
	}
	// image → image
	imageSpec := LookupResourceTypeSpec(ResourceTypeImage)
	got = ResolvePrimaryStore([]*ResourceStore{
		mkStore(StoreTypeThumbnail, 1),
		mkStore(StoreTypeImage, 2),
	}, imageSpec)
	if got == nil || got.StoreType != StoreTypeImage {
		t.Fatalf("image 期望取 image, 实际 %+v", got)
	}
	// nil spec(未知/历史 resource_type)→ 降级取 image
	got = ResolvePrimaryStore([]*ResourceStore{
		mkStore(StoreTypeThumbnail, 1),
		mkStore(StoreTypeImage, 2),
		mkStore(StoreTypeDocument, 3),
	}, nil)
	if got == nil || got.StoreType != StoreTypeImage {
		t.Fatalf("nil spec 降级期望取 image, 实际 %+v", got)
	}
	// nil spec 有 merged(历史 video)→ 降级 merged 优先(完整可播放,优于无音频的 videoTrack)
	got = ResolvePrimaryStore([]*ResourceStore{
		mkStore(StoreTypeVideoTrack, 1),
		mkStore(StoreTypeAudioTrack, 2),
		mkStore(StoreTypeMerged, 3),
	}, nil)
	if got == nil || got.StoreType != StoreTypeMerged {
		t.Fatalf("nil spec 有 merged 降级期望取 merged, 实际 %+v", got)
	}
	// nil spec 无 image → 降级取首个非 thumbnail
	got = ResolvePrimaryStore([]*ResourceStore{
		mkStore(StoreTypeThumbnail, 1),
		mkStore(StoreTypeDocument, 2),
	}, nil)
	if got == nil || got.StoreType != StoreTypeDocument {
		t.Fatalf("nil spec 无 image 期望取首个非 thumbnail, 实际 %+v", got)
	}
	// 空列表 → nil
	got = ResolvePrimaryStore(nil, imageSpec)
	if got != nil {
		t.Fatalf("空列表期望 nil, 实际 %+v", got)
	}
	// unknown spec(PrimaryRoles 空)→ 降级
	unknownSpec := LookupResourceTypeSpec(ResourceTypeUnknown)
	got = ResolvePrimaryStore([]*ResourceStore{mkStore(StoreTypeImage, 1)}, unknownSpec)
	if got == nil || got.StoreType != StoreTypeImage {
		t.Fatalf("unknown spec 降级期望取 image, 实际 %+v", got)
	}
}

func TestValidateResourceType(t *testing.T) {
	cases := []struct {
		rt      string
		wantErr error
	}{
		{"", ErrResourceTypeEmpty},
		{ResourceTypeImage, nil},
		{ResourceTypeVideo, nil},
		{ResourceTypeArticle, nil},
		{ResourceTypeDocument, nil},
		{ResourceTypeUnknown, nil},
		{"bogus", ErrResourceTypeInvalid},
		{"main", ErrResourceTypeInvalid},
	}
	for _, c := range cases {
		err := ValidateResourceType(c.rt)
		if err != c.wantErr {
			t.Fatalf("ValidateResourceType(%q) 期望 %v, 实际 %v", c.rt, c.wantErr, err)
		}
	}
}

func TestValidateStoreType(t *testing.T) {
	valid := []string{StoreTypeImage, StoreTypeDocument, StoreTypeThumbnail, StoreTypeVideoTrack, StoreTypeAudioTrack, StoreTypeMerged}
	for _, st := range valid {
		if err := ValidateStoreType(st); err != nil {
			t.Fatalf("ValidateStoreType(%q) 期望 nil, 实际 %v", st, err)
		}
	}
	for _, st := range []string{"", "main", "bogus"} {
		if err := ValidateStoreType(st); err != ErrStoreTypeInvalid {
			t.Fatalf("ValidateStoreType(%q) 期望 ErrStoreTypeInvalid, 实际 %v", st, err)
		}
	}
}

// sameSet 无序比较两个字符串切片内容(都为空/nil 视为相等)
func sameSet(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	mb := make(map[string]struct{}, len(b))
	for _, x := range b {
		mb[x] = struct{}{}
	}
	for _, x := range a {
		if _, ok := mb[x]; !ok {
			return false
		}
	}
	return true
}
