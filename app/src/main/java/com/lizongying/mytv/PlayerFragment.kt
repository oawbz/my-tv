package com.lizongying.mytv

import android.media.AudioManager
import android.media.MediaPlayer
import android.net.Uri
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
import android.util.Log
import android.view.Gravity
import android.view.LayoutInflater
import android.view.SurfaceHolder
import android.view.View
import android.view.ViewGroup
import android.widget.FrameLayout
import androidx.fragment.app.Fragment
import androidx.media3.common.MediaItem
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.common.VideoSize
import androidx.media3.exoplayer.ExoPlayer
import com.lizongying.mytv.databinding.PlayerBinding
import com.lizongying.mytv.models.TVViewModel

/** Both engines share surfaces, overlays, source recovery and lifecycle guards. */
class PlayerFragment : Fragment(), SurfaceHolder.Callback {
    private var _binding: PlayerBinding? = null
    private var player: MediaPlayer? = null
    private var exoPlayer: ExoPlayer? = null
    private var channel: TVViewModel? = null
    private var surfaceReady = false
    private var prepared = false
    private var rendered = false
    private var buffering = false
    private var openedAt = 0L
    private val handler = Handler(Looper.getMainLooper())
    private var recoveryAttempts = 0
    private var generation = 0
    private var bufferingTimeout: Runnable? = null
    private var stablePlayback: Runnable? = null
    private var pendingRecovery: Runnable? = null
    private val hideChannelNumber = Runnable {
        _binding?.channelNumberOverlay?.visibility = View.GONE
    }

    override fun onCreateView(
        inflater: LayoutInflater, container: ViewGroup?, savedInstanceState: Bundle?
    ): View {
        val binding = PlayerBinding.inflate(inflater, container, false)
        _binding = binding
        binding.playerSurface.holder.addCallback(this)
        binding.playerView.addOnLayoutChangeListener { _, _, _, _, _, _, _, _, _ ->
            if (prepared) {
                player?.let { resizeVideo(it.videoWidth, it.videoHeight) }
                exoPlayer?.videoSize?.let { resizeVideo(it.width, it.height) }
            }
        }
        (activity as? MainActivity)?.fragmentReady("PlayerFragment")
        return binding.root
    }

    override fun surfaceCreated(holder: SurfaceHolder) {
        surfaceReady = true
        if (isResumed && player == null && exoPlayer == null && pendingRecovery == null) channel?.let(::startPlayback)
    }

    override fun surfaceChanged(holder: SurfaceHolder, format: Int, width: Int, height: Int) {}

    override fun surfaceDestroyed(holder: SurfaceHolder) {
        surfaceReady = false
        cancelRecovery()
        releasePlayer()
        playbackActive(false)
    }

    fun play(tvViewModel: TVViewModel) {
        cancelRecovery()
        recoveryAttempts = 0
        channel = tvViewModel
        startPlayback(tvViewModel)
    }

    fun restartEngine() {
        channel?.let(::play)
    }

    private fun startPlayback(current: TVViewModel) {
        cancelBufferingTimeout()
        cancelStablePlayback()
        releasePlayer()
        playbackActive(false)
        hidePlaybackError()
        showSwitching()
        if (!isResumed || !surfaceReady) return
        val binding = _binding ?: return
        val attempt = generation
        openedAt = SystemClock.elapsedRealtime()
        if (SP.useExoPlayer) {
            startExoPlayback(current, attempt)
            return
        }
        try {
            val next = MediaPlayer()
            player = next
            fun valid(mp: MediaPlayer) = player === mp && attempt == generation &&
                channel === current && isResumed && surfaceReady
            next.setAudioStreamType(AudioManager.STREAM_MUSIC)
            next.setDisplay(binding.playerSurface.holder)
            next.setScreenOnWhilePlaying(true)
            next.setOnPreparedListener { mp ->
                if (valid(mp)) {
                    prepared = true
                    Log.i(TAG, "System prepared ${current.getTV().title} in ${elapsed()}ms")
                    resizeVideo(mp.videoWidth, mp.videoHeight)
                    try { mp.start() } catch (e: IllegalStateException) {
                        Log.e(TAG, "System start failed", e)
                        handlePlaybackError()
                    }
                }
            }
            next.setOnVideoSizeChangedListener { mp, width, height ->
                if (valid(mp)) resizeVideo(width, height)
            }
            next.setOnInfoListener { mp, what, extra ->
                if (valid(mp)) {
                    Log.i(TAG, "System info=$what extra=$extra ${current.getTV().title}")
                    when (what) {
                        MediaPlayer.MEDIA_INFO_VIDEO_RENDERING_START -> {
                            rendered = true
                            _binding?.videoCover?.visibility = View.GONE
                            buffering = false
                            Log.i(TAG, "System first frame ${current.getTV().title} in ${elapsed()}ms")
                            markPlaying()
                        }
                        MediaPlayer.MEDIA_INFO_BUFFERING_START -> {
                            buffering = true
                            playbackActive(false)
                            showSwitching()
                            cancelStablePlayback()
                            scheduleBufferingTimeout()
                        }
                        MediaPlayer.MEDIA_INFO_BUFFERING_END -> {
                            buffering = false
                            if (rendered && mp.isPlaying) markPlaying()
                        }
                    }
                }
                true
            }
            next.setOnErrorListener { mp, what, extra ->
                if (valid(mp)) {
                    Log.e(TAG, "System playback error=$what extra=$extra ${current.getTV().title} after ${elapsed()}ms")
                    handlePlaybackError()
                }
                true // Suppress MediaPlayer's default error dialog/completion fallback.
            }
            next.setOnCompletionListener { mp ->
                if (valid(mp)) {
                    Log.w(TAG, "System playback ended ${current.getTV().title}")
                    handlePlaybackError()
                }
            }
            next.setDataSource(requireContext(), Uri.parse(current.getVideoUrlCurrent()))
            next.prepareAsync()
            scheduleBufferingTimeout()
        } catch (e: Exception) {
            Log.e(TAG, "System prepare failed", e)
            handlePlaybackError()
        }
    }

    private fun startExoPlayback(current: TVViewModel, attempt: Int) {
        try {
            val next = ExoPlayer.Builder(requireContext()).build()
            exoPlayer = next
            fun valid() = exoPlayer === next && attempt == generation &&
                channel === current && isResumed && surfaceReady
            fun confirm() {
                if (valid() && rendered && next.isPlaying && !buffering) markPlaying()
            }
            next.addListener(object : Player.Listener {
                override fun onRenderedFirstFrame() {
                    if (!valid()) return
                    rendered = true
                    _binding?.videoCover?.visibility = View.GONE
                    Log.i(TAG, "Exo first frame ${current.getTV().title} in ${elapsed()}ms")
                    if (next.isPlaying && !buffering) confirm()
                }
                override fun onVideoSizeChanged(size: VideoSize) {
                    if (valid()) resizeVideo(size.width, size.height)
                }
                override fun onIsPlayingChanged(playing: Boolean) {
                    if (valid() && playing && rendered && !buffering) confirm()
                }
                override fun onPlaybackStateChanged(state: Int) {
                    if (!valid()) return
                    when (state) {
                        Player.STATE_BUFFERING -> {
                            buffering = true
                            playbackActive(false)
                            showSwitching()
                            cancelStablePlayback()
                            scheduleBufferingTimeout()
                        }
                        Player.STATE_READY -> {
                            prepared = true
                            buffering = false
                            Log.i(TAG, "Exo prepared ${current.getTV().title} in ${elapsed()}ms")
                            if (rendered && next.isPlaying) confirm()
                        }
                        Player.STATE_ENDED -> handlePlaybackError()
                    }
                }
                override fun onPlayerError(error: PlaybackException) {
                    if (valid()) {
                        Log.e(TAG, "Exo playback failed", error)
                        handlePlaybackError()
                    }
                }
            })
            next.setVideoSurfaceView(_binding!!.playerSurface)
            next.setMediaItem(MediaItem.fromUri(current.getVideoUrlCurrent()))
            next.prepare()
            next.play()
            scheduleBufferingTimeout()
        } catch (e: Exception) {
            Log.e(TAG, "Exo prepare failed", e)
            handlePlaybackError()
        }
    }

    private fun elapsed() = SystemClock.elapsedRealtime() - openedAt

    private fun markPlaying() {
        cancelBufferingTimeout()
        _binding?.videoCover?.visibility = View.GONE
        hideSwitching()
        hidePlaybackError()
        playbackActive(true)
        cancelStablePlayback()
        val attempt = generation
        stablePlayback = Runnable {
            if (attempt == generation && rendered && !buffering && isResumed) recoveryAttempts = 0
        }.also { handler.postDelayed(it, STABLE_PLAYBACK_MILLIS) }
    }

    private fun resizeVideo(width: Int, height: Int) {
        val binding = _binding ?: return
        val container = binding.playerView
        if (width <= 0 || height <= 0 || container.width <= 0 || container.height <= 0) return
        val scale = minOf(container.width.toDouble() / width, container.height.toDouble() / height)
        val w = (width * scale).toInt().coerceAtLeast(1)
        val h = (height * scale).toInt().coerceAtLeast(1)
        if (binding.playerSurface.layoutParams.width != w || binding.playerSurface.layoutParams.height != h) {
            binding.playerSurface.layoutParams = FrameLayout.LayoutParams(w, h, Gravity.CENTER)
        }
    }

    private fun releasePlayer() {
        // Cover retained surface frames before releasing or replacing the decoder.
        _binding?.videoCover?.visibility = View.VISIBLE
        val oldExo = exoPlayer
        exoPlayer = null
        oldExo?.release()
        val old = player
        player = null // Invalidate callbacks before releasing a preparing/started player.
        prepared = false
        rendered = false
        buffering = false
        old?.setOnPreparedListener(null)
        old?.setOnInfoListener(null)
        old?.setOnErrorListener(null)
        old?.setOnCompletionListener(null)
        old?.setOnVideoSizeChangedListener(null)
        old?.release()
    }

    private fun handlePlaybackError() {
        cancelBufferingTimeout()
        cancelStablePlayback()
        releasePlayer()
        playbackActive(false)
        val current = channel ?: return showPlaybackError()
        pendingRecovery?.let(handler::removeCallbacks)
        pendingRecovery = null
        val delay = if (isHls(current) && recoveryAttempts < RECOVERY_DELAYS_MILLIS.size) {
            RECOVERY_DELAYS_MILLIS[recoveryAttempts++]
        } else if (current.nextSource()) {
            recoveryAttempts = 0
            0L
        } else {
            showPlaybackError()
            return
        }
        val attempt = generation
        showSwitching()
        pendingRecovery = Runnable {
            pendingRecovery = null
            if (attempt == generation && channel === current && isResumed && surfaceReady) startPlayback(current)
        }.also { handler.postDelayed(it, delay) }
    }

    private fun isHls(current: TVViewModel) =
        current.getVideoUrlCurrent().substringBefore('?').endsWith(".m3u8", ignoreCase = true)

    private fun scheduleBufferingTimeout() {
        cancelBufferingTimeout()
        val attempt = generation
        bufferingTimeout = Runnable {
            bufferingTimeout = null
            if (attempt == generation && isResumed && (player != null || exoPlayer != null) &&
                (!rendered || buffering)) {
                Log.w(TAG, "System preparation/buffering timed out after ${elapsed()}ms")
                handlePlaybackError()
            }
        }.also { handler.postDelayed(it, BUFFERING_TIMEOUT_MILLIS) }
    }

    private fun cancelBufferingTimeout() {
        bufferingTimeout?.let(handler::removeCallbacks)
        bufferingTimeout = null
    }

    private fun cancelStablePlayback() {
        stablePlayback?.let(handler::removeCallbacks)
        stablePlayback = null
    }

    private fun cancelRecovery() {
        generation++
        pendingRecovery?.let(handler::removeCallbacks)
        pendingRecovery = null
        cancelBufferingTimeout()
        cancelStablePlayback()
    }

    private fun playbackActive(active: Boolean) = (activity as? MainActivity)?.setPlaybackActive(active)
    fun showSwitching() { _binding?.switchingOverlay?.visibility = View.VISIBLE }
    private fun hideSwitching() { _binding?.switchingOverlay?.visibility = View.GONE }
    private fun showPlaybackError() {
        hideSwitching()
        playbackActive(false)
        _binding?.playbackErrorOverlay?.visibility = View.VISIBLE
    }
    private fun hidePlaybackError() { _binding?.playbackErrorOverlay?.visibility = View.GONE }

    fun showChannelNumber(number: String, hideAfterMillis: Long? = null) {
        val overlay = _binding?.channelNumberOverlay ?: return
        handler.removeCallbacks(hideChannelNumber)
        overlay.text = number
        overlay.visibility = View.VISIBLE
        hideAfterMillis?.let { handler.postDelayed(hideChannelNumber, it) }
    }

    fun hideChannelNumber() {
        handler.removeCallbacks(hideChannelNumber)
        _binding?.channelNumberOverlay?.visibility = View.GONE
    }

    override fun onResume() {
        super.onResume()
        if (surfaceReady && player == null && exoPlayer == null && pendingRecovery == null) channel?.let(::startPlayback)
    }

    override fun onPause() {
        cancelRecovery()
        releasePlayer()
        playbackActive(false)
        super.onPause()
    }

    override fun onDestroyView() {
        cancelRecovery()
        handler.removeCallbacks(hideChannelNumber)
        _binding?.playerSurface?.holder?.removeCallback(this)
        releasePlayer()
        surfaceReady = false
        (activity as? MainActivity)?.fragmentUnavailable("PlayerFragment")
        _binding = null
        super.onDestroyView()
    }

    companion object {
        private const val TAG = "PlayerFragment"
        private val RECOVERY_DELAYS_MILLIS = longArrayOf(1_500L, 3_000L, 6_000L)
        private const val BUFFERING_TIMEOUT_MILLIS = 20_000L
        private const val STABLE_PLAYBACK_MILLIS = 30_000L
    }
}
