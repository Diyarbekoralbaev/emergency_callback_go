# Audio Prompts

The voice prompts the caller hears. Six WAV files ship in the repository's
`audios/` directory and are installed into the Asterisk sounds directory.

## The prompts

| File (name used in dialplan) | Played when | Suggested content |
|------------------------------|-------------|-------------------|
| `ambulance-rating-request` | Right after the callee answers | "Please rate the ambulance service from 1 to 5." |
| `ambulance-rating-thankyou` | After a valid 1–5 rating | "Thank you, your rating is recorded." |
| `ambulance-rating-invalid` | After an invalid keypress | "Invalid choice, please press 1 to 5." |
| `ambulance-failed-rating` | Rating could not be collected | "We could not record your rating." |
| `ambulance-transfer-message` | Connecting to an operator | "Connecting you to an operator." |
| `ambulance-transfer-error` | Transfer failed | "Sorry, we could not connect you." |

The shipped recordings are in **Karakalpak** (caller-facing language). The
dialplan plays them by **bare name** (no file extension), so Asterisk auto-picks
the best matching format.

## Where they live

The default install location is the Asterisk English sounds directory. Confirm
your system's sounds path:

```bash
sudo asterisk -rx 'core show settings' | grep -i 'data directory'
# Common locations:
#   /var/lib/asterisk/sounds/en/
#   /usr/share/asterisk/sounds/en/
```

Files must be readable by the user Asterisk runs as (usually `asterisk`):

```bash
ls -l /var/lib/asterisk/sounds/en/ambulance-*.wav
# -rw-r--r-- asterisk asterisk ... ambulance-rating-request.wav
```

## Required format

- **WAV, PCM, 16-bit, mono, 8000 Hz.**

Asterisk plays this natively. If you record in another format, convert it:

```bash
# From any input to 8 kHz mono 16-bit PCM WAV:
ffmpeg -i input.mp3 -ar 8000 -ac 1 -acodec pcm_s16le ambulance-rating-request.wav
# Or with sox:
sox input.wav -r 8000 -c 1 -b 16 ambulance-rating-request.wav
```

!!! tip "Avoid resampling at call time"
    Pre-converting to 8 kHz mono avoids per-call CPU cost and quality loss.

## Replacing a prompt

1. Produce the new file in the required format with the **same base name**.
2. Copy it over the existing one and fix ownership/permissions:
   ```bash
   sudo cp ambulance-rating-request.wav /var/lib/asterisk/sounds/en/
   sudo chown asterisk:asterisk /var/lib/asterisk/sounds/en/ambulance-rating-request.wav
   sudo chmod 644 /var/lib/asterisk/sounds/en/ambulance-rating-request.wav
   ```
3. No reload is needed — `Playback` reads the file fresh on each call. The next
   call uses the new audio.
4. Keep the repo's `audios/` copy in sync so re-deploys don't revert your change:
   ```bash
   cp ambulance-rating-request.wav /path/to/emergency_callback_go/audios/
   ```

## Adding a new prompt

The audio name is just a dialplan extension played by `play-audio`. To use a new
prompt the **application must redirect to it** by name. The app's prompt names
are defined in code (`internal/ami`), so adding a brand-new prompt that the app
triggers is a code change — see
[Changing Things](../operations/changing-things.md#change-an-audio-prompt).
Re-recording the existing six requires **no** code change.

## Language / wording changes

- **Re-recording** the existing files (any language) needs no code change — same
  names, new audio.
- The **prompt-to-name mapping** (which file plays at which step) is fixed in the
  dialplan + app. Changing *which* file plays when is a dialplan/app change.
- Caller-facing **SMS** text and the **web vote page** wording are separate from
  audio — see [Changing Things](../operations/changing-things.md).
