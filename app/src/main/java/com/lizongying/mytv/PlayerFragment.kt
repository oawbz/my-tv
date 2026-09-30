package com.lizongying.mytv

import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import androidx.fragment.app.Fragment
import androidx.media3.common.MediaItem
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.exoplayer.ExoPlayer
import com.lizongying.mytv.databinding.PlayerBinding
import com.lizongying.mytv.models.TVViewModel

class PlayerFragment : Fragment() {
    private var _binding: PlayerBinding? = null
    private var player: ExoPlayer? = null
    private var channel: TVViewModel? = null
    private val channelNumberHandler = Handler(Looper.getMainLooper())
    private val recoveryHandler = Handler(Looper.getMainLooper())
    private var recoveryAttempts = 0
    private var recoveryGeneration = 0
    private var bufferingTimeout: Runnable? = null
    private var stablePlayback: Runnable? = null
    private var pendingRecovery: Runnable? = null
    private var resumePendingRecovery = false
    private val hideChannelNumber = Runnable {
        _binding?.channelNumberOverlay?.visibility = View.GONE
    }

    override fun onCreateView(
        inflater: LayoutInflater, container: ViewGroup?, savedInstanceState: Bundle?
    ): View {
        val binding = PlayerBinding.inflate(inflater, container, false)
        _binding = binding
        val newPlayer = ExoPlayer.Builder(requireContext()).build()
        player = newPlayer
        binding.playerView.player = newPlayer
        newPlayer.addListener(object : Player.Listener {
            override fun onPlayerError(error: PlaybackException) {
                Log.e(TAG, "Playback failed", error)
                handlePlaybackError()
            }

            override fun onIsPlayingChanged(isPlaying: Boolean) {
                if (isPlaying) {
                    hideSwitching()
                    scheduleStablePlayback()
                } else {
                    cancelStablePlayback()
                }
            }

            override fun onPlaybackStateChanged(playbackState: Int) {
                when (playbackState) {
                    Player.STATE_BUFFERING -> {
                        showSwitching()
                        scheduleBufferingTimeout()
                    }
                    Player.STATE_READY -> {
                        cancelBufferingTimeout()
                        hideSwitching()
                        hidePlaybackError()
                    }
                    Player.STATE_IDLE -> {
                        cancelBufferingTimeout()
                        hideSwitching()
                    }
                    Player.STATE_ENDED -> cancelBufferingTimeout()
                }
            }
        })
        (activity as MainActivity).fragmentReady("PlayerFragment")
        return binding.root
    }

    fun play(tvViewModel: TVViewModel) {
        cancelRecovery()
        recoveryAttempts = 0
        resumePendingRecovery = false
        channel = tvViewModel
        startPlayback(tvViewModel)
    }

    private fun startPlayback(tvViewModel: TVViewModel) {
        hidePlaybackError()
        player?.apply {
            setMediaItem(MediaItem.fromUri(tvViewModel.getVideoUrlCurrent()))
            prepare()
            if (this@PlayerFragment.isResumed) {
                play()
                if (playbackState == Player.STATE_BUFFERING) scheduleBufferingTimeout()
            }
        }
    }

    fun showSwitching() {
        _binding?.switchingOverlay?.visibility = View.VISIBLE
    }

    fun showChannelNumber(number: String, hideAfterMillis: Long? = null) {
        val overlay = _binding?.channelNumberOverlay ?: return
        channelNumberHandler.removeCallbacks(hideChannelNumber)
        overlay.text = number
        overlay.visibility = View.VISIBLE
        hideAfterMillis?.let { channelNumberHandler.postDelayed(hideChannelNumber, it) }
    }

    fun hideChannelNumber() {
        channelNumberHandler.removeCallbacks(hideChannelNumber)
        _binding?.channelNumberOverlay?.visibility = View.GONE
    }

    private fun showPlaybackError() {
        _binding?.playbackErrorOverlay?.visibility = View.VISIBLE
    }

    private fun hidePlaybackError() {
        _binding?.playbackErrorOverlay?.visibility = View.GONE
    }

    private fun hideSwitching() {
        _binding?.switchingOverlay?.visibility = View.GONE
    }

    private fun handlePlaybackError() {
        val current = channel
        if (current == null) {
            showPlaybackError()
            return
        }
        cancelBufferingTimeout()
        cancelStablePlayback()
        pendingRecovery?.let(recoveryHandler::removeCallbacks)
        pendingRecovery = null
        if (isHls(current) && recoveryAttempts < MAX_RECOVERY_ATTEMPTS) {
            val delay = RECOVERY_DELAYS_MILLIS[recoveryAttempts++]
            val generation = recoveryGeneration
            player?.stop()
            showSwitching()
            val retry = Runnable {
                pendingRecovery = null
                if (generation == recoveryGeneration && isResumed && channel === current) {
                    startPlayback(current)
                }
            }
            pendingRecovery = retry
            recoveryHandler.postDelayed(retry, delay)
            return
        }
        if (current.nextSource()) {
            cancelRecovery()
            recoveryAttempts = 0
            showSwitching()
            startPlayback(current)
            return
        }
        hideSwitching()
        showPlaybackError()
    }

    private fun isHls(current: TVViewModel): Boolean =
        current.getVideoUrlCurrent().substringBefore('?').endsWith(".m3u8", ignoreCase = true)

    private fun scheduleBufferingTimeout() {
        cancelBufferingTimeout()
        val current = channel ?: return
        if (!isHls(current) || player?.playWhenReady != true || !isResumed) return
        val generation = recoveryGeneration
        val timeout = Runnable {
            bufferingTimeout = null
            if (generation == recoveryGeneration && channel === current && isResumed &&
                player?.playbackState == Player.STATE_BUFFERING && player?.playWhenReady == true
            ) {
                Log.w(TAG, "HLS buffering timed out; reopening channel")
                handlePlaybackError()
            }
        }
        bufferingTimeout = timeout
        recoveryHandler.postDelayed(timeout, BUFFERING_TIMEOUT_MILLIS)
    }

    private fun cancelBufferingTimeout() {
        bufferingTimeout?.let(recoveryHandler::removeCallbacks)
        bufferingTimeout = null
    }

    private fun scheduleStablePlayback() {
        cancelStablePlayback()
        val generation = recoveryGeneration
        val reset = Runnable {
            stablePlayback = null
            if (generation == recoveryGeneration && player?.isPlaying == true) {
                recoveryAttempts = 0
            }
        }
        stablePlayback = reset
        recoveryHandler.postDelayed(reset, STABLE_PLAYBACK_MILLIS)
    }

    private fun cancelStablePlayback() {
        stablePlayback?.let(recoveryHandler::removeCallbacks)
        stablePlayback = null
    }

    private fun cancelRecovery() {
        recoveryGeneration++
        pendingRecovery?.let(recoveryHandler::removeCallbacks)
        pendingRecovery = null
        cancelBufferingTimeout()
        cancelStablePlayback()
    }

    override fun onResume() {
        super.onResume()
        if (resumePendingRecovery) {
            resumePendingRecovery = false
            channel?.let(::startPlayback)
        } else {
            player?.takeIf { it.mediaItemCount > 0 }?.play()
        }
        if (player?.playbackState == Player.STATE_BUFFERING) scheduleBufferingTimeout()
    }

    override fun onPause() {
        resumePendingRecovery = pendingRecovery != null
        cancelRecovery()
        player?.pause()
        super.onPause()
    }

    override fun onDestroyView() {
        cancelRecovery()
        channelNumberHandler.removeCallbacks(hideChannelNumber)
        (activity as? MainActivity)?.fragmentUnavailable("PlayerFragment")
        _binding?.playerView?.player = null
        player?.release()
        player = null
        _binding = null
        super.onDestroyView()
    }

    companion object {
        private const val TAG = "PlayerFragment"
        private const val MAX_RECOVERY_ATTEMPTS = 3
        private val RECOVERY_DELAYS_MILLIS = longArrayOf(1_500L, 3_000L, 6_000L)
        private const val BUFFERING_TIMEOUT_MILLIS = 20_000L
        private const val STABLE_PLAYBACK_MILLIS = 30_000L
    }
}
