# Real licensed film clips with the media learning core

This example imports short intervals from two official Blender open films, trains a small sample-specific sequence regressor through `learning.Trainer.StepFrom`, creates a new frozen `learning.Network` for `PredictAll`, and evaluates decoded RGB frames and the audio embedded in each source MP4. The existing `tasks/media.VideoGenerator` fixture contract remains unchanged and continues to use synthetic `RenderVideo` targets.

The repository contains only this manifest, code, and documentation. Keep source movies and all run outputs outside Git. The importer verifies each MP4 SHA-256, probes its dimensions, duration, codecs and audio-track metadata, rejects unsafe paths, and applies bounded FFmpeg transforms. It requires `ffmpeg` and `ffprobe` on `PATH`.

## Run

Place the verified source MP4 files directly in an external data directory, then choose a new output directory that does not already exist and is separate from the data directory:

```sh
go run ./examples/realmedia \
  --data-root /path/to/TSK-11/blender-open-movies \
  --out-dir /tmp/coimnet-realmedia-run-01
```

The command writes the exact manifest, trained snapshot, JSON report, predicted PNG sequence, 8 kHz mono WAV, synchronized H.264/AAC preview MP4, and a frame/audio timeline. It refuses an existing output directory. The preview MP4 is enlarged from the model's 8×8 pixels for easier viewing; enlargement does not add image detail. The report lists SHA-256 fingerprints for the manifest, model, and each media artifact; the command prints the report's own SHA-256 separately.

The manifest's expected source fingerprints are:

| Source | MP4 SHA-256 | License and required excerpt credit |
| --- | --- | --- |
| Elephants Dream teaser | `5fe4e42a4c6893f40c90d841ac3e638d7b68d31b04d4a91f22b2bb5071d8c2d6` | [CC BY 2.5](https://orange.blender.org/press/); `(c) copyright 2006, Blender Foundation / Netherlands Media Art Institute / www.elephantsdream.org` ([attribution terms](https://orange.blender.org/page/2/)) |
| Big Buck Bunny | `ae51005850b0ff757fe60c3dd7a12d754d3cd2397d87d939b55235e457f97658` | [CC BY 3.0](https://peach.blender.org/about/); `(c) copyright 2008, Blender Foundation / www.bigbuckbunny.org` |

The manifest selects Elephants Dream's first AAC stream and Big Buck Bunny's first embedded MP3 stream. Big Buck Bunny also contains a separate six-channel AC-3 stream, which is excluded. Its video stream starts at PTS 66.667 ms and the selected audio stream at 0 ms; the clip starts at 58,067 ms, and the importer passes that same global `-ss` to both FFmpeg decode commands. The recorded PTS difference is a stream-start metadata offset, not a claim of constant whole-clip audio/video desynchronization. No separately distributed Elephants Dream soundtrack file is used; those extra files have a different NC-ND exception.

## Evaluation limits

The clips are manually annotated visual descriptions of their exact intervals. The model encodes those descriptions as signed hashed unigrams/bigrams plus time-position features; it is not a language model. The split uses two Elephants Dream training intervals and one same-source validation interval, with one Big Buck Bunny interval as the independent-source test. This small test is not broad generalization evidence.

Frames are reduced to 8×8 RGB at 4 fps. Embedded audio is downmixed to mono 8 kHz, with 2,000 samples aligned to each 250 ms frame. Scores are clipped-output image/audio MSE and do not measure perceived quality, caption correctness, or semantic audio/video synchronization. The run report must be read for actual results; a completed import/train/infer/export path alone does not mean the generated media passed content evaluation.
