package main

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin/plugintest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestIsHEICFile(t *testing.T) {
	tests := []struct {
		name     string
		info     model.FileInfo
		expected bool
	}{
		{"heic extension", model.FileInfo{Name: "IMG_0001.HEIC", Extension: "HEIC"}, true},
		{"heif extension", model.FileInfo{Name: "a.heif", Extension: "heif"}, true},
		{"extension from name only", model.FileInfo{Name: "a.heic"}, true},
		{"mime only", model.FileInfo{Name: "photo", MimeType: "image/heic"}, true},
		{"jpeg", model.FileInfo{Name: "a.jpg", Extension: "jpg", MimeType: "image/jpeg"}, false},
		{"png", model.FileInfo{Name: "a.png", Extension: "png", MimeType: "image/png"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isHEICFile(&tt.info))
		})
	}
}

func TestHEICRenamed(t *testing.T) {
	assert.Equal(t, "IMG_0001.jpg", heicRenamed("IMG_0001.HEIC", "jpg"))
	assert.Equal(t, "my.photo.jpg", heicRenamed("my.photo.heif", "jpg"))
	assert.Equal(t, "image.png", heicRenamed(".heic", "png"))
}

func TestConvertHEICFile_PassesThroughMislabelledFiles(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))

	var jpg bytes.Buffer
	require.NoError(t, jpeg.Encode(&jpg, img, nil))
	out, ext, reencoded, err := convertHEICFile(jpg.Bytes())
	require.NoError(t, err)
	assert.Equal(t, "jpg", ext)
	assert.False(t, reencoded)
	assert.Equal(t, jpg.Bytes(), out, "JPEG should be passed through unchanged")

	var pngBuf bytes.Buffer
	require.NoError(t, png.Encode(&pngBuf, img))
	out, ext, reencoded, err = convertHEICFile(pngBuf.Bytes())
	require.NoError(t, err)
	assert.Equal(t, "png", ext)
	assert.False(t, reencoded)
	assert.Equal(t, pngBuf.Bytes(), out)

	heicData, err := os.ReadFile("testdata/sample.heic")
	require.NoError(t, err)
	out, ext, reencoded, err = convertHEICFile(heicData)
	require.NoError(t, err)
	assert.Equal(t, "jpg", ext)
	assert.True(t, reencoded)
	_, err = jpeg.Decode(bytes.NewReader(out))
	assert.NoError(t, err)
}

func TestConvertHEICToJPEG(t *testing.T) {
	data, err := os.ReadFile("testdata/sample.heic")
	require.NoError(t, err)

	out, err := convertHEICToJPEG(data)
	require.NoError(t, err)

	img, err := jpeg.Decode(bytes.NewReader(out))
	require.NoError(t, err)
	assert.Positive(t, img.Bounds().Dx())
	assert.Positive(t, img.Bounds().Dy())

	_, err = convertHEICToJPEG([]byte("not an image"))
	assert.Error(t, err)
}

func TestConvertHEICAttachments(t *testing.T) {
	heicData, err := os.ReadFile("testdata/sample.heic")
	require.NoError(t, err)

	setup := func(t *testing.T, enabled bool) (*Plugin, *plugintest.API) {
		p, api, _ := newTestPlugin(t, nil)
		p.setConfiguration(&configuration{ConvertHEIC: enabled})
		api.On("GetFileInfo", "heic-own").Return(&model.FileInfo{Id: "heic-own", Name: "IMG_1.HEIC", Extension: "heic", CreatorId: "user1", Size: int64(len(heicData))}, nil).Maybe()
		api.On("GetFileInfo", "heic-other").Return(&model.FileInfo{Id: "heic-other", Name: "IMG_2.HEIC", Extension: "heic", CreatorId: "user2"}, nil).Maybe()
		api.On("GetFileInfo", "jpg-own").Return(&model.FileInfo{Id: "jpg-own", Name: "a.jpg", Extension: "jpg", CreatorId: "user1"}, nil).Maybe()
		api.On("GetFile", "heic-own").Return(heicData, nil).Maybe()
		api.On("UploadFile", mock.AnythingOfType("[]uint8"), "ch1", "IMG_1.jpg").Return(&model.FileInfo{Id: "jpg-new", Name: "IMG_1.jpg"}, nil).Maybe()
		api.On("SendEphemeralPost", "user1", mock.AnythingOfType("*model.Post")).Return(func(_ string, post *model.Post) *model.Post {
			sent := post.Clone()
			sent.Id = "notice1"
			return sent
		}).Maybe()
		return p, api
	}

	t.Run("converts poster's own HEIC and leaves other files", func(t *testing.T) {
		p, api := setup(t, true)
		post := &model.Post{UserId: "user1", ChannelId: "ch1", FileIds: model.StringArray{"jpg-own", "heic-own"}}
		p.convertHEICAttachments(post)
		assert.Equal(t, model.StringArray{"jpg-own", "jpg-new"}, post.FileIds)
		api.AssertCalled(t, "UploadFile", mock.AnythingOfType("[]uint8"), "ch1", "IMG_1.jpg")
		v, tracked := p.convertedHEICFiles.Load("jpg-new")
		require.True(t, tracked, "converted file should be tracked for post refresh")
		notice := v.(*heicNotice)
		assert.Equal(t, "notice1", notice.post.Id, "poster should get a please-wait notice")
		assert.Contains(t, notice.post.Message, "please wait")
		require.Len(t, notice.results, 1)
		assert.Contains(t, notice.results[0], "Converted **IMG_1.HEIC** to **IMG_1.jpg**")
	})

	t.Run("does not convert another user's file", func(t *testing.T) {
		p, api := setup(t, true)
		post := &model.Post{UserId: "user1", ChannelId: "ch1", FileIds: model.StringArray{"heic-other"}}
		p.convertHEICAttachments(post)
		assert.Equal(t, model.StringArray{"heic-other"}, post.FileIds)
		api.AssertNotCalled(t, "GetFile", "heic-other")
		api.AssertNotCalled(t, "SendEphemeralPost", mock.Anything, mock.Anything)
	})

	t.Run("disabled by config", func(t *testing.T) {
		p, api := setup(t, false)
		post := &model.Post{UserId: "user1", ChannelId: "ch1", FileIds: model.StringArray{"heic-own"}}
		p.convertHEICAttachments(post)
		assert.Equal(t, model.StringArray{"heic-own"}, post.FileIds)
		api.AssertNotCalled(t, "GetFileInfo", "heic-own")
	})
}

func TestMessageHasBeenPosted_RefreshesConvertedPosts(t *testing.T) {
	orig := heicRefreshDelay
	heicRefreshDelay = 0
	defer func() { heicRefreshDelay = orig }()

	p, api, _ := newTestPlugin(t, nil)
	saved := &model.Post{Id: "post1", ChannelId: "ch1", FileIds: model.StringArray{"jpg-new"}}
	refreshed := make(chan struct{})
	api.On("GetPost", "post1").Return(saved, nil)
	api.On("UpdatePost", saved).Run(func(mock.Arguments) { close(refreshed) }).Return(saved, nil)

	// A post without converted files is left alone
	p.MessageHasBeenPosted(nil, &model.Post{Id: "post2", FileIds: model.StringArray{"other"}})

	noticeUpdated := make(chan string, 1)
	api.On("UpdateEphemeralPost", "user1", mock.AnythingOfType("*model.Post")).Run(func(args mock.Arguments) {
		noticeUpdated <- args.Get(1).(*model.Post).Message
	}).Return(func(_ string, post *model.Post) *model.Post { return post })

	notice := &heicNotice{userID: "user1", post: &model.Post{Id: "notice1"}, results: []string{"✅ Converted **a.HEIC** to **a.jpg**."}}
	p.convertedHEICFiles.Store("jpg-new", notice)
	p.MessageHasBeenPosted(nil, saved)

	select {
	case <-refreshed:
	case <-time.After(2 * time.Second):
		t.Fatal("post was not refreshed")
	}
	select {
	case msg := <-noticeUpdated:
		assert.Contains(t, msg, "Converted **a.HEIC** to **a.jpg**")
	case <-time.After(2 * time.Second):
		t.Fatal("notice was not updated")
	}
	api.AssertNotCalled(t, "GetPost", "post2")
	_, stillTracked := p.convertedHEICFiles.Load("jpg-new")
	assert.False(t, stillTracked)
}

func TestConvertHEICAttachments_FailureReportsImmediately(t *testing.T) {
	p, api, _ := newTestPlugin(t, nil)
	p.setConfiguration(&configuration{ConvertHEIC: true})
	api.On("GetFileInfo", "bad").Return(&model.FileInfo{Id: "bad", Name: "broken.heic", Extension: "heic", CreatorId: "user1"}, nil)
	api.On("GetFile", "bad").Return([]byte("not an image"), nil)
	api.On("SendEphemeralPost", "user1", mock.AnythingOfType("*model.Post")).Return(func(_ string, post *model.Post) *model.Post {
		sent := post.Clone()
		sent.Id = "notice1"
		return sent
	})
	var final string
	api.On("UpdateEphemeralPost", "user1", mock.AnythingOfType("*model.Post")).Run(func(args mock.Arguments) {
		final = args.Get(1).(*model.Post).Message
	}).Return(func(_ string, post *model.Post) *model.Post { return post })

	post := &model.Post{UserId: "user1", ChannelId: "ch1", FileIds: model.StringArray{"bad"}}
	p.convertHEICAttachments(post)

	assert.Equal(t, model.StringArray{"bad"}, post.FileIds, "original stays attached")
	assert.Contains(t, final, "Couldn't convert **broken.heic**")
	_, tracked := p.convertedHEICFiles.Load("bad")
	assert.False(t, tracked)
}
