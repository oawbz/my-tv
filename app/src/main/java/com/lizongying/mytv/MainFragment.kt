package com.lizongying.mytv

import android.os.Bundle
import android.util.Log
import android.app.AlertDialog
import android.view.View
import android.view.ViewGroup
import androidx.leanback.app.BrowseSupportFragment
import androidx.leanback.widget.ArrayObjectAdapter
import androidx.leanback.widget.BaseGridView
import androidx.leanback.widget.HeaderItem
import androidx.leanback.widget.ListRow
import androidx.leanback.widget.ListRowPresenter
import androidx.leanback.widget.ListRowPresenter.SelectItemViewHolderTask
import androidx.leanback.widget.OnItemViewClickedListener
import androidx.leanback.widget.OnItemViewSelectedListener
import androidx.leanback.widget.Presenter
import androidx.leanback.widget.Row
import androidx.leanback.widget.RowPresenter
import androidx.leanback.widget.VerticalGridView
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
            (activity as? MainActivity)?.updateMobileChannels(mobileGroups())
            if (onResult != null) SP.channelsUrl = url
            (activity as MainActivity).fragmentReady("MainFragment")
            if (onResult != null && hadChannels) {
                tvViewModels.getOrNull(itemPosition)?.let(::playChannel)
            }
            onResult?.invoke(true)
        }
    }

    private fun playChannel(tvViewModel: TVViewModel) {
        if (!isAdded || view == null || tvViewModel !in tvViewModels) return
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

        val cardPresenter = CardPresenter(::playChannelFromTouch)

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
                playChannelFromTouch(item)
            }
        }
    }

    private fun playChannelFromTouch(channel: TVViewModel) {
        itemPosition = tvViewModels.indexOf(channel).takeIf { it >= 0 } ?: itemPosition
        playChannel(channel)
        (activity as? MainActivity)?.hideChannelList()
    }

    private inner class ItemViewSelectedListener : OnItemViewSelectedListener {
        override fun onItemSelected(
            itemViewHolder: Presenter.ViewHolder?, item: Any?,
            rowViewHolder: RowPresenter.ViewHolder, row: Row
        ) {
            if (item is TVViewModel) {
                itemPosition = tvViewModels.indexOf(item).takeIf { it >= 0 } ?: itemPosition
                (activity as MainActivity).mainActive()
            }
        }
    }

    fun focusPlayingChannel() {
        val playingPosition = tvViewModels.indexOfFirst {
            channelKey(it.getTV()) == SP.selectedChannel
        }
        if (playingPosition >= 0) itemPosition = playingPosition
        focusChannelAt(itemPosition)
    }

    fun moveChannelSelection(direction: Int) {
        view?.post {
            if (tvViewModels.isEmpty()) return@post
            val playingPosition = tvViewModels.indexOfFirst {
                channelKey(it.getTV()) == SP.selectedChannel
            }
            val currentPosition = if (isHidden || playingPosition < 0) {
                playingPosition.takeIf { it >= 0 } ?: itemPosition
            } else {
                itemPosition
            }
            itemPosition = (currentPosition + direction).coerceIn(0, tvViewModels.lastIndex)
            (activity as? MainActivity)?.showChannelList()
            focusChannelAt(itemPosition)
        }
    }

    fun playSelectedChannel() {
        val selected = tvViewModels.getOrNull(itemPosition) ?: return
        playChannel(selected)
        (activity as? MainActivity)?.hideChannelList()
    }

    fun playMobileChannel(channel: TVViewModel) {
        val position = tvViewModels.indexOf(channel)
        if (position < 0) return
        itemPosition = position
        playChannel(channel)
    }

    private fun mobileGroups(): List<Pair<String, List<TVViewModel>>> =
        tvViewModels.groupBy { it.getTV().channel }.map { it.key to it.value }

    private fun focusChannelAt(position: Int) {
        val channel = tvViewModels.getOrNull(position) ?: return
        val rowPosition = channel.getRowPosition()
        val itemPositionInRow = channel.getItemPosition()
        val task = SelectItemViewHolderTask(itemPositionInRow)
        setSelectedPosition(rowPosition, true, task)

        // Keep one category row visible above the selected channel when possible, so the
        // user can immediately navigate upward from the default selection.
        if (rowPosition > 0) {
            view?.post {
                val grid = findVerticalGridView(getRowsSupportFragment()?.view) ?: return@post
                val rowView = grid.findViewHolderForAdapterPosition(rowPosition)?.itemView
                    ?: return@post
                if (rowView.height <= 0) return@post
                grid.setWindowAlignment(BaseGridView.WINDOW_ALIGN_NO_EDGE)
                grid.setWindowAlignmentOffsetPercent(-1f)
                grid.setWindowAlignmentOffset(rowView.height + grid.verticalSpacing)
                setSelectedPosition(rowPosition, true, task)
            }
        }
    }

    private fun findVerticalGridView(view: View?): VerticalGridView? {
        if (view is VerticalGridView) return view
        if (view is ViewGroup) {
            for (index in 0 until view.childCount) {
                findVerticalGridView(view.getChildAt(index))?.let { return it }
            }
        }
        return null
    }

    fun fragmentReady() {
        tvViewModels.getOrNull(itemPosition)?.let(::playChannel)
    }

    fun prev() {
        view?.post {
            if (tvViewModels.isEmpty()) return@post
            val playingPosition = tvViewModels.indexOfFirst {
                channelKey(it.getTV()) == SP.selectedChannel
            }.takeIf { it >= 0 } ?: itemPosition
            val previousPosition = (playingPosition + tvViewModels.size - 1) % tvViewModels.size
            if (previousPosition == playingPosition) return@post
            itemPosition = previousPosition
            playChannel(tvViewModels[itemPosition])
        }
    }

    fun next() {
        view?.post {
            if (tvViewModels.isEmpty()) return@post
            val playingPosition = tvViewModels.indexOfFirst {
                channelKey(it.getTV()) == SP.selectedChannel
            }.takeIf { it >= 0 } ?: itemPosition
            val nextPosition = (playingPosition + 1) % tvViewModels.size
            if (nextPosition == playingPosition) return@post
            itemPosition = nextPosition
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
