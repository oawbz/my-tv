package com.lizongying.mytv

import android.os.Bundle
import android.util.Log
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.Toast
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
                if (isPlaying) hideSwitching()
            }
        })
        (activity as MainActivity).fragmentReady("PlayerFragment")
        return binding.root
    }

    fun play(tvViewModel: TVViewModel) {
        channel = tvViewModel
        player?.apply {
            setMediaItem(MediaItem.fromUri(tvViewModel.getVideoUrlCurrent()))
            prepare()
            if (this@PlayerFragment.isResumed) play()
        }
    }

    fun showSwitching() {
        _binding?.switchingOverlay?.visibility = View.VISIBLE
    }

    private fun hideSwitching() {
        _binding?.switchingOverlay?.visibility = View.GONE
    }

    private fun handlePlaybackError() {
        val current = channel
        if (current != null && current.nextSource()) {
            showSwitching()
            play(current)
        } else {
            hideSwitching()
            Toast.makeText(context, "播放失败，请切换频道", Toast.LENGTH_SHORT).show()
        }
    }

    override fun onResume() {
        super.onResume()
        player?.takeIf { it.mediaItemCount > 0 }?.play()
    }

    override fun onPause() {
        player?.pause()
        super.onPause()
    }

    override fun onDestroyView() {
        (activity as? MainActivity)?.fragmentUnavailable("PlayerFragment")
        _binding?.playerView?.player = null
        player?.release()
        player = null
        _binding = null
        super.onDestroyView()
    }

    companion object {
        private const val TAG = "PlayerFragment"
    }
}
