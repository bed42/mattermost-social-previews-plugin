package main

import (
	"bytes"
	"fmt"
	"image/jpeg"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gen2brain/heic"
	"github.com/mattermost/mattermost/server/public/model"
	"github.com/mattermost/mattermost/server/public/plugin"
)

// maxHEICSize caps the size of HEIC files we'll attempt to convert, to bound
// decode memory. iPhone photos are typically 1-5MB.
const maxHEICSize = 50 * 1024 * 1024

var heicExtensions = map[string]bool{"heic": true, "heif": true, "hif": true}

var heicMimeTypes = map[string]bool{
	"image/heic":          true,
	"image/heif":          true,
	"image/heic-sequence": true,
	"image/heif-sequence": true,
}

// isHEICFile reports whether a file looks like a HEIC/HEIF image by extension or MIME type.
func isHEICFile(info *model.FileInfo) bool {
	ext := strings.ToLower(strings.TrimPrefix(info.Extension, "."))
	if ext == "" {
		ext = strings.ToLower(strings.TrimPrefix(filepath.Ext(info.Name), "."))
	}
	return heicExtensions[ext] || heicMimeTypes[strings.ToLower(info.MimeType)]
}

// heicRenamed swaps a HEIC filename's extension for ext (IMG_0001.HEIC -> IMG_0001.jpg).
func heicRenamed(name, ext string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if base == "" {
		base = "image"
	}
	return base + "." + ext
}

// convertHEICFile returns browser-displayable bytes for a file named/typed as
// HEIC, the extension to give it, and whether it had to be re-encoded. iOS
// sometimes saves JPEGs (or PNGs) under a .heic name; those are passed through
// unchanged and only renamed, since the extension is all that stops web/desktop
// clients showing them.
func convertHEICFile(data []byte) (out []byte, ext string, reencoded bool, err error) {
	switch http.DetectContentType(data) {
	case "image/jpeg":
		return data, "jpg", false, nil
	case "image/png":
		return data, "png", false, nil
	}
	out, err = convertHEICToJPEG(data)
	return out, "jpg", true, err
}

// heicJPEGQuality balances size against fidelity for photos; at 90 the result
// is visually indistinguishable and typically 1.5-3x the HEIC size.
const heicJPEGQuality = 90

// convertHEICToJPEG decodes a HEIC image (orientation already applied by the
// decoder) and re-encodes it as JPEG. EXIF metadata, including GPS, is not carried over.
func convertHEICToJPEG(data []byte) ([]byte, error) {
	img, err := heic.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode HEIC: %w", err)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: heicJPEGQuality}); err != nil {
		return nil, fmt.Errorf("failed to encode JPEG: %w", err)
	}
	return buf.Bytes(), nil
}

// heicNotice tracks the ephemeral status message shown to the poster while
// their HEIC attachments are converted, and the outcome to report once done.
type heicNotice struct {
	userID  string
	post    *model.Post
	results []string
}

// sendHEICNotice posts (or updates) the poster-only status message.
func (p *Plugin) sendHEICNotice(n *heicNotice, message string) {
	if n.post == nil {
		return
	}
	n.post.Message = message
	if n.post.Id == "" {
		n.post = p.API.SendEphemeralPost(n.userID, n.post)
		return
	}
	n.post = p.API.UpdateEphemeralPost(n.userID, n.post)
}

// heicNoticeSummary renders the final status message from per-file results.
func heicNoticeSummary(results []string) string {
	return strings.Join(results, "\n") + "\n\n_Only you can see this message._"
}

// convertHEICAttachments replaces any HEIC attachments on a post with JPEG
// copies, so web and desktop clients (which can't render HEIC) show them
// inline. Failures leave the original file attached. The poster sees an
// ephemeral "please wait" message, updated with the outcome once done.
//
// This runs in MessageWillBePosted rather than FileWillBeUploaded because the
// server ignores a replacement FileInfo returned from FileWillBeUploaded on the
// streaming upload path, which would leave JPEG bytes stored under a .heic
// name with no thumbnail.
func (p *Plugin) convertHEICAttachments(post *model.Post) {
	if !p.getConfiguration().ConvertHEIC || len(post.FileIds) == 0 {
		return
	}

	var notice *heicNotice
	swapped := []string{}

	for i, fileID := range post.FileIds {
		info, appErr := p.API.GetFileInfo(fileID)
		if appErr != nil || info == nil || !isHEICFile(info) {
			continue
		}

		// Only convert the poster's own, not-yet-attached uploads. The server
		// enforces this when attaching files; since the JPEG copy is uploaded by
		// the plugin, we must enforce it ourselves or a post could leak someone
		// else's file.
		if info.CreatorId != post.UserId || info.PostId != "" || info.DeleteAt != 0 {
			p.API.LogWarn("SOCIAL PREVIEWS: Skipping HEIC conversion for file not owned by poster", "file_id", fileID, "user_id", post.UserId)
			continue
		}

		if notice == nil {
			notice = &heicNotice{
				userID: post.UserId,
				post:   &model.Post{ChannelId: post.ChannelId, RootId: post.RootId},
			}
			p.sendHEICNotice(notice, "⏳ Converting HEIC image for web and desktop, please wait...")
		}

		fail := func(reason string) {
			notice.results = append(notice.results, fmt.Sprintf("⚠️ Couldn't convert **%s**: %s. The original was attached instead.", info.Name, reason))
		}

		if info.Size > maxHEICSize {
			p.API.LogWarn("SOCIAL PREVIEWS: HEIC file too large to convert", "file_id", fileID, "size", info.Size)
			fail("file is too large")
			continue
		}

		data, appErr := p.API.GetFile(fileID)
		if appErr != nil {
			p.API.LogWarn("SOCIAL PREVIEWS: Failed to read HEIC file", "file_id", fileID, "error", appErr.Error())
			fail("couldn't read the file")
			continue
		}

		converted, ext, reencoded, err := convertHEICFile(data)
		if err != nil {
			p.API.LogWarn("SOCIAL PREVIEWS: Failed to convert HEIC file", "file_id", fileID, "error", err.Error())
			fail("the image couldn't be decoded")
			continue
		}

		newName := heicRenamed(info.Name, ext)
		newInfo, appErr := p.API.UploadFile(converted, post.ChannelId, newName)
		if appErr != nil {
			p.API.LogWarn("SOCIAL PREVIEWS: Failed to upload converted image", "file_id", fileID, "error", appErr.Error())
			fail("the converted image couldn't be saved")
			continue
		}

		p.API.LogInfo("SOCIAL PREVIEWS: Converted HEIC attachment", "file_id", fileID, "new_file_id", newInfo.Id, "name", newInfo.Name, "reencoded", reencoded)
		post.FileIds[i] = newInfo.Id
		swapped = append(swapped, newInfo.Id)
		if reencoded {
			notice.results = append(notice.results, fmt.Sprintf("✅ Converted **%s** to **%s**.", info.Name, newName))
		} else {
			notice.results = append(notice.results, fmt.Sprintf("✅ Renamed **%s** to **%s** (it was already a %s).", info.Name, newName, strings.ToUpper(ext)))
		}
	}

	if notice == nil {
		return
	}
	if len(swapped) == 0 {
		// Nothing to refresh, so report the outcome now.
		p.sendHEICNotice(notice, heicNoticeSummary(notice.results))
		return
	}
	for _, id := range swapped {
		p.convertedHEICFiles.Store(id, notice)
	}
}

// heicRefreshDelay gives the poster's client time to process the create-post
// response before the refresh arrives; otherwise it can re-apply its stale copy.
var heicRefreshDelay = 2 * time.Second

// MessageHasBeenPosted refreshes posts whose HEIC attachments were swapped.
//
// The poster's client renders its own post from the files it uploaded and
// never picks up the server's file list, so it keeps showing the original
// .heic until reload. Re-saving the post unchanged broadcasts a post_edited
// event with the real attachments. Since neither the message nor the file
// list changes, the server doesn't mark the post as edited.
func (p *Plugin) MessageHasBeenPosted(c *plugin.Context, post *model.Post) {
	var notice *heicNotice
	for _, id := range post.FileIds {
		if v, ok := p.convertedHEICFiles.LoadAndDelete(id); ok {
			notice = v.(*heicNotice)
		}
	}
	if notice == nil {
		return
	}

	postID := post.Id
	time.AfterFunc(heicRefreshDelay, func() {
		p.refreshPost(postID)
		p.sendHEICNotice(notice, heicNoticeSummary(notice.results))
	})
}

// refreshPost re-saves a post unchanged so clients receive its current state.
func (p *Plugin) refreshPost(postID string) {
	current, appErr := p.API.GetPost(postID)
	if appErr != nil {
		p.API.LogWarn("SOCIAL PREVIEWS: Failed to load post for refresh", "post_id", postID, "error", appErr.Error())
		return
	}
	if _, appErr := p.API.UpdatePost(current); appErr != nil {
		p.API.LogWarn("SOCIAL PREVIEWS: Failed to refresh post after HEIC conversion", "post_id", postID, "error", appErr.Error())
		return
	}
	p.API.LogInfo("SOCIAL PREVIEWS: Refreshed post after HEIC conversion", "post_id", postID)
}
