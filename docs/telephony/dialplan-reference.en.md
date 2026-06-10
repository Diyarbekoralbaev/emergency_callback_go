# Dialplan Reference

The application controls calls through Asterisk by **originating** a Local
channel and then issuing **Redirect** actions over AMI. Four contexts are
involved. On FreePBX, `from-internal` already exists (do not redefine it); you
add the other three.

## How the legs fit together

When the worker originates `Local/<number>@from-internal/n` with
`Context=ambulance-callback`, Asterisk creates a Local channel with two halves:

- **`;1` leg** runs `from-internal` → your outbound route → the trunk → the
  callee's phone.
- **`;2` leg** is the control leg → runs `ambulance-callback,s` **after the
  callee answers**.

The two legs are bridged, so the caller hears whatever the `;2` leg plays. The
`/n` suffix keeps the Local channel from "optimizing away", which is required so
the app can `Redirect` the `;2` leg into `play-audio` after answer.

Variables set at originate time (inherited to both legs):

| Variable | Meaning |
|----------|---------|
| `CALL_ID` | The app's internal UUID for this call. Echoed back in every `UserEvent` so the app can match events to the call. |
| `PHONE_NUMBER` | The original phone number. |
| `BRIGADE_ID` | The team ID. |
| `CALLBACK_REQUEST_ID` | The DB row ID of the callback. |

---

## `from-internal` (FreePBX-owned)

You do **not** define this on FreePBX — it is FreePBX's standard context that
applies your Outbound Routes and sends the call to the trunk. The app dials the
**stripped local number** into it.

On a **standalone** Asterisk (no FreePBX) you must provide it yourself; see
[Standalone Asterisk](standalone-asterisk.md).

---

## `ambulance-callback`

The control leg. Runs only after the callee answers.

```ini
[ambulance-callback]
exten => s,1,NoOp(ANSWERED CALL_ID=${CALL_ID} PHONE=${PHONE_NUMBER})
 same => n,Answer()
 same => n,UserEvent(CallAnswered,CallID: ${CALL_ID},Phone: ${PHONE_NUMBER})
 same => n,Wait(300)
 same => n,Hangup()
```

| Line | Purpose |
|------|---------|
| `Answer()` | Answers the control leg. |
| `UserEvent(CallAnswered,…)` | Fires an AMI event the app listens for. The app captures **this channel** as the one to redirect, and starts the rating prompt. |
| `Wait(300)` | Keeps the leg alive while the app drives it via Redirects. The app, not this line, decides when the call ends. |

---

## `play-audio`

Plays a named prompt, then waits for DTMF. The app redirects the call here with
the **extension set to the audio name** (e.g. `ambulance-rating-request`).

```ini
[play-audio]
exten => _.,1,NoOp(PLAY ${EXTEN} CALL_ID=${CALL_ID})
 same => n,Playback(${EXTEN})
 same => n,UserEvent(AudioPlayed,CallID: ${CALL_ID},Audio: ${EXTEN})
 same => n,WaitExten(60)
 same => n,Wait(60)
 same => n,Hangup()
```

| Line | Purpose |
|------|---------|
| `_.` pattern | Matches any extension — i.e. any audio name. |
| `Playback(${EXTEN})` | Plays the WAV whose name equals the extension. |
| `WaitExten(60)` / `Wait(60)` | Keeps the leg up so the caller can press keys; the app catches the `DTMFEnd` events. The app usually redirects again (to thank-you) or hangs up before these elapse. |

The audio names the app uses: `ambulance-rating-request`,
`ambulance-rating-thankyou`, `ambulance-rating-invalid`. See
[Audio Prompts](audio-prompts.md).

---

## `transfer-to-337`

Where the call goes when the caller chooses to be transferred to an operator
(presses `0` or `9`).

```ini
[transfer-to-337]
exten => s,1,NoOp(TRANSFER CALL_ID=${CALL_ID})
 same => n,Dial(Local/337@from-internal,30)
 same => n,Hangup()
```

Change the `Dial(...)` target to your operator destination (extension, ring
group, or queue). The context name is historical (`337`); the **name does not
have to match the destination** — only the app's redirect target must match this
context name.

!!! note "The context name is a contract"
    The app redirects to the literal context names `play-audio` and
    `transfer-to-337`, and originates into `ambulance-callback`. If you rename a
    context in the dialplan, the app will redirect into a non-existent context
    and the call breaks. Keep these three names exactly as shown (renaming them
    requires an application change).

---

## What the app sends (AMI actions)

| Moment | AMI action |
|--------|-----------|
| Start the call | `Originate` Local channel into `ambulance-callback` |
| Play any prompt | `Redirect` the answered channel → `play-audio` ext `<audio-name>` |
| Transfer | `Redirect` → `transfer-to-337` ext `s` |
| End the call | `Hangup` the channel |

## What the app listens for (AMI events)

| Event | Used for |
|-------|----------|
| `Newchannel` | Initial channel discovery (fallback). |
| `UserEvent: CallAnswered` | Marks answer; captures the control channel; starts the rating prompt. |
| `DTMFEnd` | Caller keypresses (rating 1–5, transfer 0/9). Duplicate digits arriving on multiple bridge legs are de-duplicated. |
| `Hangup` | Ends the call. |
