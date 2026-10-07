package com.lizongying.mytv

import android.os.Bundle
import android.util.Log
import android.app.AlertDialog
import androidx.fragment.app.Fragment
import androidx.lifecycle.lifecycleScope
import com.lizongying.mytv.models.TVViewModel
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/** Channel loading and playback selection; no channel-list views are created here. */
class MainFragment : Fragment() {

    private var itemPosition = 0
    private var loadingErrorDialog: AlertDialog? = null
    private var loadJob: Job? = null
    private var loadGeneration = 0L

    private val tvViewModels = mutableListOf<TVViewModel>()

    override fun onCreate(savedInstanceState: Bundle?) {
        Log.i(TAG, "onCreate")
        super.onCreate(savedInstanceState)
        loadChannels()
    }

    fun reloadChannels(url: String, onResult: (Boolean) -> Unit) {
        loadingErrorDialog?.dismiss()
        loadingErrorDialog = null
        loadChannels(url, onResult)
    }

    private fun loadChannels(
        url: String = SP.channelsUrl,
        onResult: ((Boolean) -> Unit)? = null,
    ) {
        loadJob?.cancel()
        val generation = ++loadGeneration
        loadJob = lifecycleScope.launch {
            val channels = try {
                withContext(Dispatchers.IO) { ChannelDataRepository.loadChannels(url) }
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                if (generation != loadGeneration) return@launch
                Log.e(TAG, "Unable to load channels", e)
                if (onResult != null) {
                    onResult(false)
                } else if (isAdded) {
                    loadingErrorDialog = AlertDialog.Builder(requireContext())
                        .setMessage("频道列表加载失败，请检查网络后重试")
                        .setPositiveButton("重试") { _, _ ->
                            loadingErrorDialog = null
                            loadChannels()
                        }
                        .setNegativeButton("退出") { _, _ -> activity?.finish() }
                        .setCancelable(false)
                        .show()
                }
                return@launch
            }
            if (generation != loadGeneration) return@launch
            val hadChannels = tvViewModels.isNotEmpty()
            loadChannelModels(channels)
            (activity as? MainActivity)?.updateChannelGroups(channelGroups())
            if (onResult != null) SP.channelsUrl = url
            (activity as MainActivity).fragmentReady("MainFragment")
            if (onResult != null && hadChannels) {
                tvViewModels.getOrNull(itemPosition)?.let(::playChannel)
            }
            onResult?.invoke(true)
        }
    }

    private fun playChannel(tvViewModel: TVViewModel) {
        if (!isAdded || activity?.isFinishing != false || tvViewModel !in tvViewModels) return
        tvViewModel.resetSource()
        SP.selectedChannel = channelKey(tvViewModel.getTV())
        Log.i(TAG, "switch ${tvViewModel.getTV().title}")
        (activity as? MainActivity)?.showChannelSwitching()
        (activity as? MainActivity)?.play(tvViewModel)
        (activity as? MainActivity)?.showInfoFragment(tvViewModel)
    }

    private fun loadChannelModels(channels: Map<String, List<TV>>) {
        tvViewModels.clear()
        for (entries in channels.values) {
            tvViewModels.addAll(entries.map(::TVViewModel))
        }

        itemPosition = tvViewModels.indexOfFirst {
            channelKey(it.getTV()) == SP.selectedChannel
        }.takeIf { it >= 0 } ?: SP.itemPosition
        if (itemPosition !in tvViewModels.indices) {
            itemPosition = 0
        }
    }

    fun playChannelFromList(channel: TVViewModel) {
        val position = tvViewModels.indexOf(channel)
        if (position < 0) return
        itemPosition = position
        playChannel(channel)
    }

    private fun channelGroups(): List<Pair<String, List<TVViewModel>>> =
        tvViewModels.groupBy { it.getTV().channel }.map { it.key to it.value }

    fun fragmentReady() {
        tvViewModels.getOrNull(itemPosition)?.let(::playChannel)
    }

    fun prev() {
        if (tvViewModels.isEmpty()) return
        val playingPosition = tvViewModels.indexOfFirst {
            channelKey(it.getTV()) == SP.selectedChannel
        }.takeIf { it >= 0 } ?: itemPosition
        val previousPosition = (playingPosition + tvViewModels.size - 1) % tvViewModels.size
        if (previousPosition == playingPosition) return
        itemPosition = previousPosition
        playChannel(tvViewModels[itemPosition])
    }

    fun next() {
        if (tvViewModels.isEmpty()) return
        val playingPosition = tvViewModels.indexOfFirst {
            channelKey(it.getTV()) == SP.selectedChannel
        }.takeIf { it >= 0 } ?: itemPosition
        val nextPosition = (playingPosition + 1) % tvViewModels.size
        if (nextPosition == playingPosition) return
        itemPosition = nextPosition
        playChannel(tvViewModels[itemPosition])
    }

    fun selectChannelNumber(number: Int) {
        val index = number - 1
        if (index !in tvViewModels.indices) return
        itemPosition = index
        playChannel(tvViewModels[itemPosition])
        (activity as? MainActivity)?.hideChannelList()
    }

    override fun onResume() {
        Log.i(TAG, "onResume")
        super.onResume()
    }

    override fun onStop() {
        Log.i(TAG, "onStop")
        super.onStop()
        SP.itemPosition = itemPosition
        Log.i(TAG, "$POSITION $itemPosition saved")
    }

    override fun onDestroy() {
        loadGeneration++
        loadJob?.cancel()
        loadJob = null
        (activity as? MainActivity)?.fragmentUnavailable("MainFragment")
        loadingErrorDialog?.dismiss()
        loadingErrorDialog = null
        super.onDestroy()
    }

    private fun channelKey(tv: TV): String = "${tv.channel.length}:${tv.channel}${tv.title}"

    companion object {
        private const val TAG = "MainFragment"
        private const val POSITION = "position"
    }
}
