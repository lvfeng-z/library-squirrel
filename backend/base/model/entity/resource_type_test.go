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
	// 6 个内置类型全部注册(含 audio)
	expected := []string{ResourceTypeImage, ResourceTypeVideo, ResourceTypeArticle, ResourceTypeDocument, ResourceTypeAudio, ResourceTypeUnknown}
	for _, rt := range expected {
		spec, ok := ResourceTypeRegistry.Lookup(rt)
		if !ok {
			t.Fatalf("内置类型 %s 未注册", rt)
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
	uk, _ := ResourceTypeRegistry.Lookup(ResourceTypeUnknown)
	if uk.Roles != nil || uk.PrimaryRoles != nil {
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
			// 本地封装视频形态:单一可播放主体 videoMain(downloaded)
			name:   "video 本地封装完整(仅 videoMain)",
			rt:     ResourceTypeVideo,
			counts: map[string]int{StoreTypeVideoMain: 1},
		},
		{
			// 远程分离流形态:videoTrack+audioTrack 原料 + videoMain(derived 合并产物)
			name:   "video 分离流完整(videoTrack+audioTrack+videoMain)",
			rt:     ResourceTypeVideo,
			counts: map[string]int{StoreTypeVideoTrack: 1, StoreTypeAudioTrack: 1, StoreTypeVideoMain: 1},
		},
		{
			// 分离流未合并:缺可播放主体 videoMain(前端提示用户合并)
			name:     "video 分离流未合并缺 videoMain",
			rt:       ResourceTypeVideo,
			counts:   map[string]int{StoreTypeVideoTrack: 1, StoreTypeAudioTrack: 1},
			wantMiss: []string{StoreTypeVideoMain},
		},
		{
			// audioTrack 可选:缺 audioTrack 不再报缺失(分离流原料可缺其一)
			name:   "video 缺 audioTrack 不报缺失(原料可选)",
			rt:     ResourceTypeVideo,
			counts: map[string]int{StoreTypeVideoTrack: 1, StoreTypeVideoMain: 1},
		},
		{
			name:       "video videoMain 超量(2>Max1)",
			rt:         ResourceTypeVideo,
			counts:     map[string]int{StoreTypeVideoMain: 2},
			wantExcess: []string{StoreTypeVideoMain},
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
			name:     "article 缺正文 document",
			rt:       ResourceTypeArticle,
			counts:   map[string]int{StoreTypeImage: 3},
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
	// video 有 videoMain → videoMain
	videoSpec := LookupResourceTypeSpec(ResourceTypeVideo)
	got := ResolvePrimaryStore([]*ResourceStore{
		mkStore(StoreTypeVideoTrack, 1),
		mkStore(StoreTypeAudioTrack, 2),
		mkStore(StoreTypeVideoMain, 3),
	}, videoSpec)
	if got == nil || got.StoreType != StoreTypeVideoMain {
		t.Fatalf("video 有 videoMain 期望取 videoMain, 实际 %+v", got)
	}
	// video 无 videoMain(分离流未合并)→ 降级 videoTrack(无声播放,决策3 暂保留)
	got = ResolvePrimaryStore([]*ResourceStore{
		mkStore(StoreTypeVideoTrack, 1),
		mkStore(StoreTypeAudioTrack, 2),
	}, videoSpec)
	if got == nil || got.StoreType != StoreTypeVideoTrack {
		t.Fatalf("video 无 videoMain 降级期望取 videoTrack, 实际 %+v", got)
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
	// nil spec 有 videoMain(历史 video)→ 降级 videoMain 优先(完整可播放,优于无音频的 videoTrack)
	got = ResolvePrimaryStore([]*ResourceStore{
		mkStore(StoreTypeVideoTrack, 1),
		mkStore(StoreTypeAudioTrack, 2),
		mkStore(StoreTypeVideoMain, 3),
	}, nil)
	if got == nil || got.StoreType != StoreTypeVideoMain {
		t.Fatalf("nil spec 有 videoMain 降级期望取 videoMain, 实际 %+v", got)
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
		{ResourceTypeAudio, nil},
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
	valid := []string{StoreTypeImage, StoreTypeDocument, StoreTypeThumbnail, StoreTypeVideoTrack, StoreTypeAudioTrack, StoreTypeVideoMain, StoreTypeAudioMain}
	for _, st := range valid {
		if err := ValidateStoreType(st); err != nil {
			t.Fatalf("ValidateStoreType(%q) 期望 nil, 实际 %v", st, err)
		}
	}
	for _, st := range []string{"", "main", "bogus", "merged"} {
		if err := ValidateStoreType(st); err != ErrStoreTypeInvalid {
			t.Fatalf("ValidateStoreType(%q) 期望 ErrStoreTypeInvalid, 实际 %v", st, err)
		}
	}
}

func TestAudioResourceTypeSpec(t *testing.T) {
	// audio 完整:audioMain + thumbnail
	miss, excess := ValidateResourceStructure(ResourceTypeAudio, map[string]int{StoreTypeAudioMain: 1, StoreTypeThumbnail: 1})
	if len(miss) != 0 || len(excess) != 0 {
		t.Fatalf("audio 完整期望无缺失无超量, 实际 miss=%v excess=%v", miss, excess)
	}
	// audio 缺 audioMain(可播放主体)→ 报缺失
	miss, _ = ValidateResourceStructure(ResourceTypeAudio, map[string]int{StoreTypeThumbnail: 1})
	if !sameSet(miss, []string{StoreTypeAudioMain}) {
		t.Fatalf("audio 缺 audioMain 期望报缺失, 实际 miss=%v", miss)
	}
	// ComputeResourceComplete:audio 完整=1,缺主体=2
	if c, _, _ := ComputeResourceComplete(ResourceTypeAudio, map[string]int{StoreTypeAudioMain: 1}); c != 1 {
		t.Fatalf("audio 完整期望 complete=1, 实际 %d", c)
	}
	if c, _, _ := ComputeResourceComplete(ResourceTypeAudio, map[string]int{}); c != 2 {
		t.Fatalf("audio 缺主体期望 complete=2, 实际 %d", c)
	}
	// PrimaryStore 取 audioMain
	audioSpec := LookupResourceTypeSpec(ResourceTypeAudio)
	got := ResolvePrimaryStore([]*ResourceStore{
		mkStore(StoreTypeThumbnail, 1),
		mkStore(StoreTypeAudioMain, 2),
	}, audioSpec)
	if got == nil || got.StoreType != StoreTypeAudioMain {
		t.Fatalf("audio PrimaryStore 期望 audioMain, 实际 %+v", got)
	}
}

func TestRegister_Validation(t *testing.T) {
	// 合法自定义类型(反向域名前缀 + 内置角色组合)注册成功
	customSpec := ResourceTypeSpec{
		ResourceType: "com.example.interactiveNovel",
		Roles:        []StoreRoleSpec{{StoreType: StoreTypeDocument, Min: 1, Max: 1}},
		PrimaryRoles: []string{StoreTypeDocument},
	}
	if err := ResourceTypeRegistry.Register(customSpec); err != nil {
		t.Fatalf("合法自定义类型注册期望 nil, 实际 %v", err)
	}
	t.Cleanup(func() { ResourceTypeRegistry.Unregister("com.example.interactiveNovel") })
	// 注册后可 Lookup 且 ValidateResourceType 放行
	if _, ok := ResourceTypeRegistry.Lookup("com.example.interactiveNovel"); !ok {
		t.Fatal("注册后 Lookup 期望命中")
	}
	if err := ValidateResourceType("com.example.interactiveNovel"); err != nil {
		t.Fatalf("已注册自定义类型 ValidateResourceType 期望 nil, 实际 %v", err)
	}
	// 重复注册拒绝(决策7)
	if err := ResourceTypeRegistry.Register(customSpec); err != ErrResourceTypeAlreadyRegistered {
		t.Fatalf("重复注册期望 ErrResourceTypeAlreadyRegistered, 实际 %v", err)
	}
	// 内置类型不可反注册(Unregister 静默保留)
	ResourceTypeRegistry.Unregister(ResourceTypeImage)
	if _, ok := ResourceTypeRegistry.Lookup(ResourceTypeImage); !ok {
		t.Fatal("内置类型 Unregister 后仍应保留")
	}

	badCases := []struct {
		name string
		spec ResourceTypeSpec
		err  error
	}{
		{"裸名无前缀(决策8)", ResourceTypeSpec{ResourceType: "custom", Roles: []StoreRoleSpec{{StoreType: StoreTypeImage, Min: 1, Max: 1}}, PrimaryRoles: []string{StoreTypeImage}}, ErrResourceTypeInvalidPrefix},
		{"Min>Max", ResourceTypeSpec{ResourceType: "com.example.bad", Roles: []StoreRoleSpec{{StoreType: StoreTypeImage, Min: 2, Max: 1}}, PrimaryRoles: []string{StoreTypeImage}}, ErrStoreRoleMaxLessThanMin},
		{"StoreType非法", ResourceTypeSpec{ResourceType: "com.example.bad2", Roles: []StoreRoleSpec{{StoreType: "bogus", Min: 1, Max: 1}}, PrimaryRoles: []string{"bogus"}}, ErrStoreTypeInvalid},
		{"PrimaryRole不在Roles", ResourceTypeSpec{ResourceType: "com.example.bad3", Roles: []StoreRoleSpec{{StoreType: StoreTypeImage, Min: 1, Max: 1}}, PrimaryRoles: []string{StoreTypeDocument}}, ErrPrimaryRoleNotInRoles},
		{"空类型名", ResourceTypeSpec{ResourceType: "", Roles: []StoreRoleSpec{{StoreType: StoreTypeImage, Min: 1, Max: 1}}, PrimaryRoles: []string{StoreTypeImage}}, ErrResourceTypeEmpty},
	}
	for _, c := range badCases {
		t.Run(c.name, func(t *testing.T) {
			if err := ResourceTypeRegistry.Register(c.spec); err != c.err {
				t.Fatalf("期望 %v, 实际 %v", c.err, err)
			}
			// 坏 spec 不应入 Registry
			if c.spec.ResourceType != "" {
				if _, ok := ResourceTypeRegistry.Lookup(c.spec.ResourceType); ok {
					t.Fatalf("坏 spec %s 不应入 Registry", c.spec.ResourceType)
				}
			}
		})
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
