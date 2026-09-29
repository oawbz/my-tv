package com.lizongying.mytv

import android.os.Bundle
import android.util.Log
import android.app.AlertDialog
import androidx.leanback.app.BrowseSupportFragment
import androidx.leanback.widget.ArrayObjectAdapter
import androidx.leanback.widget.HeaderItem
import androidx.leanback.widget.ListRow
import androidx.leanback.widget.ListRowPresenter
import androidx.leanback.widget.ListRowPresenter.SelectItemViewHolderTask
import androidx.leanback.widget.OnItemViewClickedListener
import androidx.leanback.widget.OnItemViewSelectedListener
import androidx.leanback.widget.Presenter
import androidx.leanback.widget.Row
import androidx.leanback.widget.RowPresenter
import androidx.lifecycle.lifecycleScope
import com.lizongying.mytv.models.TVViewModel
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

class MainFragment : BrowseSupportFragment() {

    private var itemPosition = 0
    private var loadingErrorDialog: AlertDialog? = null
    private var loadJob: Job? = null
    private var loadGeneration = 0L

    private val tvViewModels = mutableListOf<TVViewModel>()

    override fun onCreate(savedInstanceState: Bundle?) {
        Log.i(TAG, "onCreate")
        super.onCreate(savedInstanceState)
        headersState = HEADERS_DISABLED
    }

    override fun onActivityCreated(savedInstanceState: Bundle?) {
        super.onActivityCreated(savedInstanceState)

        setupEventListeners()

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
        loadJob = viewLifecycleOwner.lifecycleScope.launch {
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
            loadRows(channels)
            if (onResult != null) SP.channelsUrl = url
            (activity as MainActivity).fragmentReady("MainFragment")
            if (onResult != null && hadChannels) {
                tvViewModels.getOrNull(itemPosition)?.let(::playChannel)
            }
            onResult?.invoke(true)
        }
    }

    private fun playChannel(tvViewModel: TVViewModel) {
        tvViewModel.resetSource()
        SP.selectedChannel = channelKey(tvViewModel.getTV())
        Log.i(TAG, "switch ${tvViewModel.getTV().title}")
        (activity as? MainActivity)?.showChannelSwitching()
        (activity as? MainActivity)?.play(tvViewModel)
        (activity as? MainActivity)?.showInfoFragment(tvViewModel)
        setSelectedPosition(
            tvViewModel.getRowPosition(), true,
            SelectItemViewHolderTask(tvViewModel.getItemPosition())
        )
    }

    override fun startHeadersTransition(withHeaders: Boolean) {
    }

    private fun loadRows(channels: Map<String, List<TV>>) {
        val rowsAdapter = ArrayObjectAdapter(ListRowPresenter())
        tvViewModels.clear()

        val cardPresenter = CardPresenter()

        var idx: Long = 0
        for ((k, v) in channels) {
            val listRowAdapter = ArrayObjectAdapter(cardPresenter)
            for ((idx2, v1) in v.withIndex()) {
                val tvViewModel = TVViewModel(v1)
                tvViewModel.setRowPosition(idx.toInt())
                tvViewModel.setItemPosition(idx2)
                tvViewModels.add(tvViewModel)
                listRowAdapter.add(tvViewModel)
            }
            val header = HeaderItem(idx, k)
            rowsAdapter.add(ListRow(header, listRowAdapter))
            idx++
        }

        adapter = rowsAdapter

        itemPosition = tvViewModels.indexOfFirst {
            channelKey(it.getTV()) == SP.selectedChannel
        }.takeIf { it >= 0 } ?: SP.itemPosition
        if (itemPosition !in tvViewModels.indices) {
            itemPosition = 0
        }
    }

    private fun setupEventListeners() {
        onItemViewClickedListener = ItemViewClickedListener()
        onItemViewSelectedListener = ItemViewSelectedListener()
    }

    private inner class ItemViewClickedListener : OnItemViewClickedListener {
        override fun onItemClicked(
            itemViewHolder: Presenter.ViewHolder,
            item: Any,
            rowViewHolder: RowPresenter.ViewHolder,
            row: Row
        ) {
            if (item is TVViewModel) {
                itemPosition = tvViewModels.indexOf(item)
                playChannel(item)
                (activity as? MainActivity)?.switchMainFragment()
            }
        }
    }

    private inner class ItemViewSelectedListener : OnItemViewSelectedListener {
        override fun onItemSelected(
            itemViewHolder: Presenter.ViewHolder?, item: Any?,
            rowViewHolder: RowPresenter.ViewHolder, row: Row
        ) {
            if (item is TVViewModel) {
                (activity as MainActivity).mainActive()
            }
        }
    }

    fun fragmentReady() {
        tvViewModels.getOrNull(itemPosition)?.let(::playChannel)
    }

    fun prev() {
        view?.post {
            if (tvViewModels.isEmpty()) return@post
            itemPosition = (itemPosition - 1 + tvViewModels.size) % tvViewModels.size
            playChannel(tvViewModels[itemPosition])
        }
    }

    fun next() {
        view?.post {
            if (tvViewModels.isEmpty()) return@post
            itemPosition = (itemPosition + 1) % tvViewModels.size
            playChannel(tvViewModels[itemPosition])
        }
    }

    fun selectChannelNumber(number: Int) {
        view?.post {
            val index = number - 1
            if (index !in tvViewModels.indices) return@post
            itemPosition = index
            playChannel(tvViewModels[itemPosition])
            (activity as? MainActivity)?.hideChannelList()
        }
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

    override fun onDestroyView() {
        loadGeneration++
        loadJob?.cancel()
        loadJob = null
        (activity as? MainActivity)?.fragmentUnavailable("MainFragment")
        loadingErrorDialog?.dismiss()
        loadingErrorDialog = null
        super.onDestroyView()
    }

    private fun channelKey(tv: TV): String = "${tv.channel.length}:${tv.channel}${tv.title}"

    override fun onDestroy() {
        Log.i(TAG, "onDestroy")
        super.onDestroy()
    }

    companion object {
        private const val TAG = "MainFragment"
        private const val POSITION = "position"
    }
}
