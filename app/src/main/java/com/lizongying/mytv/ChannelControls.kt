package com.lizongying.mytv

import android.graphics.Color
import android.graphics.drawable.GradientDrawable
import android.util.TypedValue
import android.view.KeyEvent
import android.view.GestureDetector
import android.view.MotionEvent
import android.view.View
import android.view.ViewGroup
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import com.lizongying.mytv.models.TVViewModel
import kotlin.math.abs
import kotlin.math.min

/** Shared vertical channel drawer for touch and TV remote controls. */
class ChannelControls(
    private val activity: MainActivity,
    private val root: View,
) {
    private val drawer: View = root.findViewById(R.id.channel_drawer)
    private val scrim: View = root.findViewById(R.id.channel_scrim)
    private val groupsView: LinearLayout = root.findViewById(R.id.channel_groups)
    private val channelsView: LinearLayout = root.findViewById(R.id.channel_items)
    private val channelScroll: ScrollView = root.findViewById(R.id.channel_scroll)
    private var groups: List<Pair<String, List<TVViewModel>>> = emptyList()
    private var selectedGroup: String? = null
    private var currentChannel: TVViewModel? = null
    init {
        val maxWidth = dp(420)
        val availableWidth = activity.resources.displayMetrics.widthPixels
        drawer.layoutParams = drawer.layoutParams.apply {
            width = min(maxWidth, (availableWidth * 0.7f).toInt())
        }
        installSurface(root.findViewById(R.id.channel_video_tap)) { openDrawer() }
        installSurface(scrim) { closeDrawer() }
        root.findViewById<View>(R.id.channel_close_drawer).setOnClickListener { closeDrawer() }
    }

    fun enableControls() {
        // Observe without consuming: both scroll columns retain native touch scrolling.
        drawer.setOnTouchListener { _, _ -> activity.channelListActive(); false }
        fun observe(view: View) {
            view.setOnTouchListener { _, _ -> activity.channelListActive(); false }
            if (view is ViewGroup) for (i in 0 until view.childCount) observe(view.getChildAt(i))
        }
        observe(drawer)
        root.visibility = View.VISIBLE
        root.findViewById<View>(R.id.channel_video_tap).visibility =
            View.VISIBLE
        root.findViewById<View>(R.id.channel_close_drawer).visibility =
            View.VISIBLE
        closeAll()
    }

    fun setGroups(value: List<Pair<String, List<TVViewModel>>>) {
        groups = value
        if (selectedGroup !in groups.map { it.first }) selectedGroup = groups.firstOrNull()?.first
        if (drawer.visibility == View.VISIBLE) renderGroups()
    }

    fun setChannel(channel: TVViewModel) {
        currentChannel = channel
        if (drawer.visibility == View.VISIBLE) {
            val playingGroup = channel.getTV().channel
            if (groups.any { it.first == playingGroup }) selectedGroup = playingGroup
            renderGroups()
            scrollCurrentIntoView()
        }
    }

    fun isDrawerOpen(): Boolean = drawer.visibility == View.VISIBLE

    fun closeOverlay(): Boolean {
        if (isDrawerOpen()) {
            closeDrawer()
            return true
        }
        return false
    }

    fun closeAll() {
        drawer.visibility = View.GONE
        scrim.visibility = View.GONE
    }

    fun openDrawer() {
        selectedGroup = currentChannel?.getTV()?.channel
            ?.takeIf { group -> groups.any { it.first == group } }
            ?: selectedGroup ?: groups.firstOrNull()?.first
        renderGroups()
        scrim.visibility = View.VISIBLE
        drawer.visibility = View.VISIBLE
        scrollCurrentIntoView()
        run {
            val channels = groups.firstOrNull { it.first == selectedGroup }?.second.orEmpty()
            val index = channels.indexOf(currentChannel).coerceAtLeast(0)
            (channelsView.getChildAt(index) ?: groupsView.getChildAt(0))?.requestFocus()
            activity.channelListActive()
        }
    }

    private fun closeDrawer() {
        drawer.visibility = View.GONE
        scrim.visibility = View.GONE
    }

    private fun renderGroups() {
        groupsView.removeAllViews()
        for ((group, _) in groups) {
            val row = row(group, group == selectedGroup)
            row.setOnClickListener {
                activity.channelListActive()
                selectedGroup = group
                renderGroups()
                channelScroll.scrollTo(0, 0)
            }
            row.setOnFocusChangeListener { view, focused ->
                if (focused) {
                    selectedGroup = group
                    renderChannels()
                    for (i in 0 until groupsView.childCount) {
                        val groupRow = groupsView.getChildAt(i) as TextView
                        paintRow(groupRow, groups[i].first == selectedGroup)
                    }
                }
                paintRow(view as TextView, group == selectedGroup)
            }
            groupsView.addView(row)
        }
        renderChannels()
    }

    private fun renderChannels() {
        channelsView.removeAllViews()
        val channels = groups.firstOrNull { it.first == selectedGroup }?.second.orEmpty()
        for (channel in channels) {
            val playing = channel === currentChannel
            val label = if (playing) "▶ ${channel.getTV().title}" else channel.getTV().title
            val row = row(label, playing)
            row.setOnClickListener {
                activity.playChannelFromList(channel)
                closeDrawer()
            }
            row.setOnFocusChangeListener { view, _ -> paintRow(view as TextView, playing) }
            channelsView.addView(row)
        }
    }

    private fun scrollCurrentIntoView() {
        val channels = groups.firstOrNull { it.first == selectedGroup }?.second.orEmpty()
        val index = channels.indexOf(currentChannel)
        if (index < 0) return
        channelScroll.post {
            val row = channelsView.getChildAt(index) ?: return@post
            val visibleTop = channelScroll.scrollY
            val visibleBottom = visibleTop + channelScroll.height
            if (row.top < visibleTop || row.bottom > visibleBottom) {
                channelScroll.smoothScrollTo(0, (row.top - channelScroll.height / 2).coerceAtLeast(0))
            }
        }
    }

    private fun installSurface(surface: View, onTap: () -> Unit) {
        var startX = 0f
        var startY = 0f
        var longPressed = false
        surface.setOnClickListener { onTap() }
        surface.setOnLongClickListener {
            closeDrawer()
            activity.openSettings()
            true
        }
        val detector = GestureDetector(activity, object : GestureDetector.SimpleOnGestureListener() {
            override fun onDown(e: MotionEvent): Boolean = true

            override fun onLongPress(e: MotionEvent) {
                longPressed = true
                surface.performLongClick()
            }
        })
        surface.setOnTouchListener { view, event ->
            if (event.actionMasked == MotionEvent.ACTION_DOWN) {
                startX = event.x
                startY = event.y
                longPressed = false
            }
            detector.onTouchEvent(event)
            if (event.actionMasked == MotionEvent.ACTION_UP && !longPressed) {
                val dx = event.x - startX
                val dy = event.y - startY
                if (abs(dy) >= dp(48) && abs(dy) > abs(dx) * 1.3f) {
                    if (dy < 0) activity.next() else activity.prev()
                } else if (abs(dx) < dp(12) && abs(dy) < dp(12)) {
                    view.performClick()
                }
            }
            true
        }
    }

    private fun row(label: String, selected: Boolean): TextView = TextView(activity).apply {
        text = label
        setTextColor(Color.WHITE)
        setTextSize(TypedValue.COMPLEX_UNIT_SP, 20f)
        gravity = android.view.Gravity.CENTER_VERTICAL
        minHeight = dp(52)
        setPadding(dp(12), dp(8), dp(8), dp(8))
        isSingleLine = true
        ellipsize = android.text.TextUtils.TruncateAt.END
        isFocusable = true
        setOnTouchListener { _, _ -> activity.channelListActive(); false }
        paintRow(this, selected)
        layoutParams = LinearLayout.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT,
            ViewGroup.LayoutParams.WRAP_CONTENT,
        )
    }

    private fun paintRow(row: TextView, selected: Boolean) {
        val focused = row.hasFocus()
        row.background = GradientDrawable().apply {
            setColor(when {
                focused -> Color.rgb(38, 105, 170)
                selected -> Color.rgb(59, 77, 99)
                else -> Color.TRANSPARENT
            })
            if (focused) setStroke(dp(2), Color.WHITE)
        }
    }

    fun handleRemoteKey(keyCode: Int): Boolean {
        if (!isDrawerOpen()) return false
        val inGroups = groupsView.hasFocus()
        val column = if (inGroups) groupsView else channelsView
        val index = (0 until column.childCount).firstOrNull {
            column.getChildAt(it).hasFocus()
        } ?: 0
        when (keyCode) {
            KeyEvent.KEYCODE_DPAD_UP, KeyEvent.KEYCODE_DPAD_DOWN -> {
                val delta = if (keyCode == KeyEvent.KEYCODE_DPAD_UP) -1 else 1
                val target = (index + delta).coerceIn(0, (column.childCount - 1).coerceAtLeast(0))
                column.getChildAt(target)?.requestFocus()
            }
            KeyEvent.KEYCODE_DPAD_LEFT -> {
                val groupIndex = groups.indexOfFirst { it.first == selectedGroup }
                groupsView.getChildAt(groupIndex.coerceAtLeast(0))?.requestFocus()
            }
            KeyEvent.KEYCODE_DPAD_RIGHT -> if (inGroups) {
                channelsView.getChildAt(0)?.requestFocus()
            }
            KeyEvent.KEYCODE_DPAD_CENTER, KeyEvent.KEYCODE_ENTER -> {
                if (inGroups) channelsView.getChildAt(0)?.requestFocus()
                else channelsView.getChildAt(index)?.performClick()
            }
            else -> return false
        }
        activity.channelListActive()
        return true
    }

    private fun dp(value: Int): Int =
        (value * activity.resources.displayMetrics.density + 0.5f).toInt()
}
