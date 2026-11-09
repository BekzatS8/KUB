package services

import (
	"archive/zip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/text/encoding/charmap"

	"turcompany/internal/models"
	"turcompany/internal/repositories"
)

// Хранилище, ТЗ «доработки CRM КУБ», п.8: загрузка папками, скачивание с
// папками (ZIP), архивировать/разархивировать, пароль на файлы и папки.

// DriveLockedError — узел (или папка выше) защищён паролем и не разблокирован.
type DriveLockedError struct {
	NodeID int64
	Name   string
}

func (e *DriveLockedError) Error() string { return fmt.Sprintf("drive: node %d is password-protected", e.NodeID) }

var (
	ErrDriveWrongPassword = errors.New("drive: wrong password")
	ErrDriveWeakPassword  = errors.New("drive: password too short")
	ErrDriveTooManyTries  = errors.New("drive: too many password attempts")
	ErrDriveBadArchive    = errors.New("drive: not a valid zip archive")
	ErrDriveArchiveTooBig = errors.New("drive: archive is too large")
	ErrDriveMixedParents  = errors.New("drive: items must be in the same folder")
)

const (
	driveUnlockTTL      = 30 * time.Minute
	driveZipLinkTTL     = 10 * time.Minute
	driveMaxZipEntries  = 5000
	driveMaxPathDepth   = 32
	drivePasswordMinLen = 4
)

// SetExtrasRepo подключает репозиторий паролей (без него пароли выключены).
func (s *DriveService) SetExtrasRepo(r *repositories.DriveExtrasRepository) { s.extras = r }

// ─── Пароли ─────────────────────────────────────────────────────────────────

type driveUnlockClaims struct {
	N int64 `json:"n"` // узел
	U int   `json:"u"` // пользователь
	E int64 `json:"e"` // истекает (unix)
}

func (s *DriveService) signJSON(v any) string {
	b, _ := json.Marshal(v)
	payload := string(b)
	enc := base64.RawURLEncoding
	return enc.EncodeToString(b) + "." + enc.EncodeToString(s.mac("x:" + payload))
}

func (s *DriveService) verifyJSON(token string, v any) bool {
	enc := base64.RawURLEncoding
	p, sig, ok := strings.Cut(strings.TrimSpace(token), ".")
	if !ok {
		return false
	}
	b, err := enc.DecodeString(p)
	if err != nil {
		return false
	}
	want, err := enc.DecodeString(sig)
	if err != nil || !hmacEqual(want, s.mac("x:"+string(b))) {
		return false
	}
	return json.Unmarshal(b, v) == nil
}

func hmacEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	var v byte
	for i := range a {
		v |= a[i] ^ b[i]
	}
	return v == 0
}

// unlockedIDs — узлы, разблокированные действующими токенами пользователя.
func (s *DriveService) unlockedIDs(actor DriveActor) map[int64]bool {
	out := map[int64]bool{}
	now := s.now().Unix()
	for _, t := range actor.UnlockTokens {
		var c driveUnlockClaims
		if s.verifyJSON(t, &c) && c.U == actor.UserID && c.E > now {
			out[c.N] = true
		}
	}
	return out
}

// ensureUnlocked — сам узел и все папки над ним либо без пароля, либо
// разблокированы пользователем. Пароль действует для всех, включая админа.
func (s *DriveService) ensureUnlocked(ctx context.Context, actor DriveActor, nodeID int64) error {
	if s.extras == nil {
		return nil
	}
	chain, err := s.extras.ProtectedChain(ctx, nodeID)
	if err != nil || len(chain) == 0 {
		return err
	}
	unlocked := s.unlockedIDs(actor)
	for _, n := range chain {
		if !unlocked[n.ID] {
			return &DriveLockedError{NodeID: n.ID, Name: n.Name}
		}
	}
	return nil
}

// markProtected проставляет признак «под паролем» в списке.
func (s *DriveService) markProtected(ctx context.Context, items []models.DriveNode) []models.DriveNode {
	if s.extras == nil || len(items) == 0 {
		return items
	}
	ids := make([]int64, 0, len(items))
	for _, n := range items {
		ids = append(ids, n.ID)
	}
	prot, err := s.extras.ProtectedAmong(ctx, ids)
	if err != nil {
		log.Printf("[drive] protected flags: %v", err)
		return items
	}
	for i := range items {
		items[i].Protected = prot[items[i].ID]
	}
	return items
}

// Простая защита от перебора: 5 неверных попыток на узел за 10 минут.
type driveAttempts struct {
	mu sync.Mutex
	m  map[string][]time.Time
}

var drivePasswordAttempts = &driveAttempts{m: map[string][]time.Time{}}

func (a *driveAttempts) allow(key string, now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	recent := a.m[key][:0]
	for _, t := range a.m[key] {
		if now.Sub(t) < 10*time.Minute {
			recent = append(recent, t)
		}
	}
	a.m[key] = recent
	return len(recent) < 5
}

func (a *driveAttempts) fail(key string, now time.Time) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.m[key] = append(a.m[key], now)
}

// Unlock проверяет пароль узла и выдаёт токен разблокировки на 30 минут.
func (s *DriveService) Unlock(ctx context.Context, actor DriveActor, id int64, password string) (string, time.Time, error) {
	if s.extras == nil {
		return "", time.Time{}, ErrDriveNotFound
	}
	if err := s.ensureAccess(ctx, actor, id); err != nil {
		return "", time.Time{}, err
	}
	hash, err := s.extras.PasswordHash(ctx, id)
	if err != nil {
		return "", time.Time{}, ErrDriveNotFound
	}
	expires := s.now().Add(driveUnlockTTL)
	if hash == "" {
		return s.signJSON(driveUnlockClaims{N: id, U: actor.UserID, E: expires.Unix()}), expires, nil
	}
	key := fmt.Sprintf("%d:%d", actor.UserID, id)
	if !drivePasswordAttempts.allow(key, s.now()) {
		return "", time.Time{}, ErrDriveTooManyTries
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		drivePasswordAttempts.fail(key, s.now())
		return "", time.Time{}, ErrDriveWrongPassword
	}
	return s.signJSON(driveUnlockClaims{N: id, U: actor.UserID, E: expires.Unix()}), expires, nil
}

// SetPassword ставит, меняет (newPassword != "") или снимает (newPassword ==
// "") пароль. Может тот, кто вправе менять узел; если пароль уже стоит, нужен
// текущий — кроме администратора (он сбрасывает забытый пароль).
func (s *DriveService) SetPassword(ctx context.Context, actor DriveActor, id int64, current, newPassword string) error {
	if s.extras == nil {
		return ErrDriveForbidden
	}
	node, err := s.getNode(ctx, id)
	if err != nil {
		return err
	}
	if err := s.canModify(ctx, actor, node); err != nil {
		return err
	}
	hash, err := s.extras.PasswordHash(ctx, id)
	if err != nil {
		return ErrDriveNotFound
	}
	if hash != "" && !actor.canManage() {
		key := fmt.Sprintf("%d:%d", actor.UserID, id)
		if !drivePasswordAttempts.allow(key, s.now()) {
			return ErrDriveTooManyTries
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(current)) != nil {
			drivePasswordAttempts.fail(key, s.now())
			return ErrDriveWrongPassword
		}
	}
	newHash := ""
	if newPassword != "" {
		if utf8.RuneCountInString(newPassword) < drivePasswordMinLen {
			return ErrDriveWeakPassword
		}
		b, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		newHash = string(b)
	}
	return s.extras.SetPassword(ctx, id, newHash, actor.UserID)
}

// ─── Загрузка папками ───────────────────────────────────────────────────────

// EnsureFolderPath создаёт (или находит) вложенные папки path внутри parent и
// возвращает последнюю — для загрузки папки вместе с файлами.
func (s *DriveService) EnsureFolderPath(ctx context.Context, actor DriveActor, parentID *int64, relPath string) (*models.DriveNode, error) {
	if s.extras == nil {
		return nil, ErrDriveForbidden
	}
	if err := s.canWriteInto(ctx, actor, parentID); err != nil {
		return nil, err
	}
	if parentID != nil {
		if err := s.ensureUnlocked(ctx, actor, *parentID); err != nil {
			return nil, err
		}
	}
	parts := splitDrivePath(relPath)
	if len(parts) == 0 {
		return nil, ErrDriveBadName
	}
	current := parentID
	var last *models.DriveNode
	for _, part := range parts {
		node, err := s.ensureChildFolder(ctx, actor, current, part)
		if err != nil {
			return nil, err
		}
		id := node.ID
		current = &id
		last = node
	}
	return s.decorateOne(*last, actor.canManage()), nil
}

func (s *DriveService) ensureChildFolder(ctx context.Context, actor DriveActor, parentID *int64, name string) (*models.DriveNode, error) {
	name, err := NormalizeDriveName(name)
	if err != nil {
		return nil, err
	}
	id, kind, err := s.extras.ChildByName(ctx, parentID, name)
	if err != nil {
		return nil, err
	}
	if id != 0 {
		if kind != models.DriveKindFolder {
			return nil, ErrDriveNameTaken
		}
		return s.getNode(ctx, id)
	}
	node := &models.DriveNode{ParentID: parentID, Kind: models.DriveKindFolder, Name: name, CreatedBy: &actor.UserID}
	if err := s.repo.CreateNode(ctx, node); err != nil {
		if errors.Is(err, repositories.ErrDriveNameTaken) {
			// Параллельная загрузка уже создала эту папку.
			if id, kind, e2 := s.extras.ChildByName(ctx, parentID, name); e2 == nil && id != 0 && kind == models.DriveKindFolder {
				return s.getNode(ctx, id)
			}
		}
		return nil, mapDriveRepoErr(err)
	}
	return node, nil
}

func splitDrivePath(p string) []string {
	p = strings.ReplaceAll(p, "\\", "/")
	out := []string{}
	for _, part := range strings.Split(p, "/") {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			continue
		}
		out = append(out, part)
	}
	if len(out) > driveMaxPathDepth {
		return nil
	}
	return out
}

// ─── Скачивание с папками (ZIP) ─────────────────────────────────────────────

type driveZipEntry struct {
	path string // путь внутри архива; у папок — с «/» на конце
	node repositories.DriveSubtreeNode
}

// collectZipEntries — выбранные узлы со всем содержимым, с проверкой доступа
// и паролей (включая защищённые папки внутри выбранных).
func (s *DriveService) collectZipEntries(ctx context.Context, actor DriveActor, ids []int64) ([]driveZipEntry, int64, error) {
	ids, err := normalizeDriveIDs(ids)
	if err != nil {
		return nil, 0, err
	}
	unlocked := s.unlockedIDs(actor)
	entries := []driveZipEntry{}
	var total int64
	usedRoot := map[string]int{}
	for _, id := range ids {
		if err := s.ensureAccess(ctx, actor, id); err != nil {
			return nil, 0, err
		}
		if err := s.ensureUnlocked(ctx, actor, id); err != nil {
			return nil, 0, err
		}
		subtree, err := s.repo.Subtree(ctx, id)
		if err != nil {
			return nil, 0, err
		}
		if len(subtree) == 0 {
			return nil, 0, ErrDriveNotFound
		}
		if s.extras != nil {
			subIDs := make([]int64, 0, len(subtree))
			for _, n := range subtree {
				subIDs = append(subIDs, n.ID)
			}
			prot, err := s.extras.ProtectedAmong(ctx, subIDs)
			if err != nil {
				return nil, 0, err
			}
			for _, n := range subtree {
				if prot[n.ID] && !unlocked[n.ID] {
					return nil, 0, &DriveLockedError{NodeID: n.ID, Name: n.Name}
				}
			}
		}
		paths := map[int64]string{}
		for _, n := range subtree {
			name := n.Name
			var p string
			if n.Depth == 0 {
				// Одинаковые имена у выбранных — «имя (2)».
				usedRoot[strings.ToLower(name)]++
				if k := usedRoot[strings.ToLower(name)]; k > 1 {
					name = numberedDriveName(name, k)
				}
				p = name
			} else {
				p = paths[*n.ParentID] + "/" + name
			}
			paths[n.ID] = p
			if n.Kind == models.DriveKindFolder {
				entries = append(entries, driveZipEntry{path: p + "/", node: n})
			} else {
				entries = append(entries, driveZipEntry{path: p, node: n})
				total += n.SizeBytes
			}
			if len(entries) > driveMaxZipEntries {
				return nil, 0, ErrDriveArchiveTooBig
			}
		}
	}
	return entries, total, nil
}

func (s *DriveService) writeZip(ctx context.Context, w io.Writer, entries []driveZipEntry) error {
	zw := zip.NewWriter(w)
	for _, e := range entries {
		hdr := &zip.FileHeader{Name: e.path, Method: zip.Deflate, Modified: s.now()}
		if e.node.Kind == models.DriveKindFolder {
			hdr.Method = zip.Store
			if _, err := zw.CreateHeader(hdr); err != nil {
				return err
			}
			continue
		}
		fw, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		r, _, err := s.store.Open(ctx, e.node.StorageKey)
		if err != nil {
			return fmt.Errorf("open %s: %w", e.path, err)
		}
		_, err = io.Copy(fw, r)
		_ = r.Close()
		if err != nil {
			return err
		}
	}
	return zw.Close()
}

type driveZipClaims struct {
	I []int64  `json:"i"`
	U int      `json:"u"`
	R int      `json:"r"`
	T []string `json:"t,omitempty"` // токены разблокировки
	N string   `json:"n"`
	E int64    `json:"e"`
}

// ZipLink — подписанная ссылка на ZIP выбранных файлов и папок (браузер
// скачивает напрямую, без загрузки в память страницы). Права и пароли
// проверяются сразу и ещё раз при скачивании.
func (s *DriveService) ZipLink(ctx context.Context, actor DriveActor, ids []int64) (*DriveLink, error) {
	if _, _, err := s.collectZipEntries(ctx, actor, ids); err != nil {
		return nil, err
	}
	name := "Хранилище.zip"
	if len(ids) == 1 {
		if n, err := s.getNode(ctx, ids[0]); err == nil {
			name = strings.TrimSuffix(n.Name, ".zip") + ".zip"
		}
	}
	expires := s.now().Add(driveZipLinkTTL)
	token := s.signJSON(driveZipClaims{I: ids, U: actor.UserID, R: actor.RoleID, T: actor.UnlockTokens, N: name, E: expires.Unix()})
	return &DriveLink{URL: "/api/v1/drive/zip/" + token, ExpiresAt: expires}, nil
}

// OpenZip пишет архив по подписанной ссылке.
func (s *DriveService) OpenZip(ctx context.Context, token string, w io.Writer, before func(name string)) error {
	var c driveZipClaims
	if !s.verifyJSON(token, &c) {
		return ErrDriveLinkInvalid
	}
	if c.E < s.now().Unix() {
		return ErrDriveLinkExpired
	}
	actor := DriveActor{UserID: c.U, RoleID: c.R, UnlockTokens: c.T}
	entries, _, err := s.collectZipEntries(ctx, actor, c.I)
	if err != nil {
		return err
	}
	before(c.N)
	return s.writeZip(ctx, w, entries)
}

// ─── Архивировать / разархивировать ─────────────────────────────────────────

// Archive упаковывает выбранные узлы (из одной папки) в ZIP рядом с ними.
func (s *DriveService) Archive(ctx context.Context, actor DriveActor, ids []int64, name string) (*models.DriveNode, error) {
	ids, err := normalizeDriveIDs(ids)
	if err != nil {
		return nil, err
	}
	var parent *int64
	for i, id := range ids {
		n, err := s.getNode(ctx, id)
		if err != nil {
			return nil, err
		}
		if i == 0 {
			parent = n.ParentID
			if name == "" {
				name = n.Name
				if len(ids) > 1 {
					name = "Архив"
				}
			}
		} else if !sameParent(parent, n.ParentID) {
			return nil, ErrDriveMixedParents
		}
	}
	if err := s.canWriteInto(ctx, actor, parent); err != nil {
		return nil, err
	}
	entries, total, err := s.collectZipEntries(ctx, actor, ids)
	if err != nil {
		return nil, err
	}
	if total > s.cfg.MaxUploadBytes {
		return nil, ErrDriveArchiveTooBig
	}
	tmp, err := s.driveTempFile("drive-archive-*.zip")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	if err := s.writeZip(ctx, tmp, entries); err != nil {
		return nil, err
	}
	size, err := tmp.Seek(0, io.SeekCurrent)
	if err != nil {
		return nil, err
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	archiveName := strings.TrimSuffix(strings.TrimSpace(name), ".zip") + ".zip"
	return s.Upload(ctx, actor, parent, archiveName, size, "application/zip", tmp)
}

// Unarchive распаковывает ZIP-файл в новую папку рядом с ним.
func (s *DriveService) Unarchive(ctx context.Context, actor DriveActor, id int64) (*models.DriveNode, error) {
	node, err := s.getNode(ctx, id)
	if err != nil {
		return nil, err
	}
	if node.IsFolder() {
		return nil, ErrDriveNotFile
	}
	if err := s.ensureAccess(ctx, actor, id); err != nil {
		return nil, err
	}
	if err := s.ensureUnlocked(ctx, actor, id); err != nil {
		return nil, err
	}
	if err := s.canWriteInto(ctx, actor, node.ParentID); err != nil {
		return nil, err
	}
	if node.SizeBytes > s.cfg.MaxUploadBytes {
		return nil, ErrDriveArchiveTooBig
	}
	tmp, err := s.driveTempFile("drive-unzip-*.zip")
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()
	src, _, err := s.store.Open(ctx, node.StorageKey)
	if err != nil {
		return nil, err
	}
	size, err := io.Copy(tmp, src)
	_ = src.Close()
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(tmp, size)
	if err != nil {
		return nil, ErrDriveBadArchive
	}
	if len(zr.File) > driveMaxZipEntries {
		return nil, ErrDriveArchiveTooBig
	}
	var total uint64
	for _, f := range zr.File {
		total += f.UncompressedSize64
	}
	if total > uint64(s.cfg.MaxUploadBytes) {
		return nil, ErrDriveArchiveTooBig
	}

	// Корневая папка распаковки: «имя архива», при занятом имени — «имя (2)».
	base := strings.TrimSuffix(node.Name, path.Ext(node.Name))
	if strings.TrimSpace(base) == "" {
		base = "Архив"
	}
	var root *models.DriveNode
	for attempt := 1; attempt <= 50; attempt++ {
		candidate := numberedDriveName(base, attempt)
		if id, _, err := s.extras.ChildByName(ctx, node.ParentID, candidate); err == nil && id == 0 {
			root, err = s.ensureChildFolder(ctx, actor, node.ParentID, candidate)
			if err != nil {
				return nil, err
			}
			break
		}
	}
	if root == nil {
		return nil, ErrDriveNameTaken
	}
	rootID := root.ID
	folders := map[string]int64{"": rootID}
	for _, f := range zr.File {
		name := zipEntryName(f)
		parts := splitDrivePath(name)
		if len(parts) == 0 || strings.HasPrefix(name, "__MACOSX") {
			continue
		}
		isDir := strings.HasSuffix(strings.ReplaceAll(name, "\\", "/"), "/") || f.FileInfo().IsDir()
		dirParts := parts
		if !isDir {
			dirParts = parts[:len(parts)-1]
		}
		parentID := rootID
		key := ""
		for _, p := range dirParts {
			key += "/" + strings.ToLower(p)
			if id, ok := folders[key]; ok {
				parentID = id
				continue
			}
			pid := parentID
			folder, err := s.ensureChildFolder(ctx, actor, &pid, p)
			if err != nil {
				return nil, err
			}
			folders[key] = folder.ID
			parentID = folder.ID
		}
		if isDir {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, ErrDriveBadArchive
		}
		pid := parentID
		limit := int64(f.UncompressedSize64)
		_, err = s.uploadExact(ctx, actor, &pid, parts[len(parts)-1], limit, rc)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
	}
	return s.reload(ctx, rootID, actor.canManage())
}

// uploadExact — загрузка с ровно size байт (защита от архивов, где размер в
// заголовке меньше реального содержимого).
func (s *DriveService) uploadExact(ctx context.Context, actor DriveActor, parentID *int64, name string, size int64, r io.Reader) (*models.DriveNode, error) {
	lr := &exactReader{r: io.LimitReader(r, size+1), max: size}
	node, err := s.Upload(ctx, actor, parentID, name, size, "", lr)
	if err != nil {
		return nil, err
	}
	if lr.over {
		return nil, ErrDriveBadArchive
	}
	return node, nil
}

type exactReader struct {
	r    io.Reader
	n    int64
	max  int64
	over bool
}

func (e *exactReader) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	e.n += int64(n)
	if e.n > e.max {
		e.over = true
		return n - int(e.n-e.max), io.EOF
	}
	return n, err
}

// driveTempFile — временный файл на диске хранилища (не в /tmp контейнера).
func (s *DriveService) driveTempFile(pattern string) (*os.File, error) {
	if s.cfg.TempDir != "" {
		if err := os.MkdirAll(s.cfg.TempDir, 0o755); err != nil {
			return nil, err
		}
	}
	return os.CreateTemp(s.cfg.TempDir, pattern)
}

// zipEntryName — имя файла в архиве. Архивы из Проводника Windows пишут
// кириллицу в CP866 без флага UTF-8 — перекодируем.
func zipEntryName(f *zip.File) string {
	if utf8.ValidString(f.Name) {
		return f.Name
	}
	if decoded, err := charmap.CodePage866.NewDecoder().String(f.Name); err == nil {
		return decoded
	}
	return f.Name
}

