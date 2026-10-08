package torrent

import "context"

// TorrentForCommand exposes torrentForCommand to the integration test (package torrent_test).
func (s *Service) TorrentForCommand(ctx context.Context, cmdID string) (id, name string) {
	return s.torrentForCommand(ctx, cmdID)
}
