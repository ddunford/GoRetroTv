package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/ddunford/goretrotv/internal/device/demux"
	"github.com/ddunford/goretrotv/internal/media"
	"github.com/ddunford/goretrotv/internal/multiplex"
	"github.com/ddunford/goretrotv/internal/web"
)

const (
	mediaPacketPeriod   = 1_000
	playoutPumpInterval = 1_024
)

type playoutSession struct {
	key     string
	encoder *media.Encoder
	decoder *media.Decoder
	pending []byte
}

func startPlayout(ctx context.Context, root string, selected multiplex.ProgrammePlayout) (*playoutSession, error) {
	if root == "" {
		return nil, fmt.Errorf("programme %q uses %s media but GORETROTV_MEDIA_ROOT is empty",
			selected.Programme, selected.Media.Kind)
	}
	playlist, err := media.ProbePlaylist(ctx, "ffprobe", root, selected.Media.Kind,
		media.Source{Path: selected.Media.Path, Loop: selected.Media.Loop}, selected.Duration)
	if err != nil {
		return nil, err
	}
	remaining := selected.Duration - selected.Elapsed
	encoder, err := media.StartEncoder(ctx, "ffmpeg", playlist, selected.Elapsed, remaining)
	if err != nil {
		return nil, err
	}
	// ffmpeg can release several decoded frames after one transport burst. These queues remain
	// bounded but cover that ordinary codec burst so scheduler jitter cannot halt the receiver.
	decoder, err := media.Start(ctx, media.Config{InputQueue: 256, VideoQueue: 64, AudioQueue: 256})
	if err != nil {
		_ = encoder.Close()
		return nil, err
	}
	return &playoutSession{key: playoutKey(selected), encoder: encoder, decoder: decoder}, nil
}

func playoutKey(p multiplex.ProgrammePlayout) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%t", p.Service, p.Programme, p.Media.Kind, p.Media.Path, p.Media.Loop)
}

func (p *playoutSession) pump(device *demux.Demux, now uint64, output *web.Transport) error {
	if p.pending == nil && !device.TransportPending() {
		select {
		case chunk, ok := <-p.encoder.Packets():
			if !ok {
				if fault := p.encoder.Fault(); fault != nil {
					return fault
				}
				p.decoder.Finish()
				return nil
			}
			p.pending = chunk
		default:
		}
	}
	if len(p.pending) != 0 && !device.TransportPending() {
		if err := device.ScheduleTransport(p.pending, now, mediaPacketPeriod); err != nil {
			return err
		}
		p.pending = nil
	}
	if admitted := device.TakeProgrammeTransport(); len(admitted) != 0 {
		if err := p.decoder.Enqueue(admitted); err != nil {
			return err
		}
	}
	for {
		select {
		case frame, ok := <-p.decoder.Video():
			if ok {
				output.PushVideo(frame.Sequence, frame.RGBA)
			}
		case audio, ok := <-p.decoder.Audio():
			if ok {
				output.PushAudio(audio.Sequence, audio.PCM)
			}
		default:
			if fault := p.encoder.Fault(); fault != nil {
				return fault
			}
			if fault := p.decoder.Fault(); fault != nil {
				return fault
			}
			return nil
		}
	}
}

func (p *playoutSession) close(logger *slog.Logger) {
	if p == nil {
		return
	}
	if err := p.encoder.Close(); err != nil {
		logger.Warn("close programme encoder", "err", err)
	}
	if err := p.decoder.Close(); err != nil {
		logger.Warn("close programme decoder", "err", err)
	}
}
