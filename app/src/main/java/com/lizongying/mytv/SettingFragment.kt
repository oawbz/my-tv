package com.lizongying.mytv

import android.os.Bundle
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.view.WindowManager
import android.widget.Toast
import android.widget.AdapterView
import android.widget.ArrayAdapter
import android.graphics.Color
import android.graphics.drawable.ColorDrawable
import androidx.fragment.app.DialogFragment
import com.lizongying.mytv.databinding.SettingBinding


class SettingFragment : DialogFragment() {

    private var _binding: SettingBinding? = null
    private val binding get() = _binding!!

    override fun onStart() {
        super.onStart()
        dialog?.window?.apply {
            setBackgroundDrawable(ColorDrawable(Color.TRANSPARENT))
            setLayout(
                minOf((320 * resources.displayMetrics.density).toInt(),
                    (resources.displayMetrics.widthPixels * 0.9f).toInt()),
                (resources.displayMetrics.heightPixels * 0.9f).toInt()
            )
            addFlags(WindowManager.LayoutParams.FLAG_FULLSCREEN)
            decorView.systemUiVisibility = View.SYSTEM_UI_FLAG_HIDE_NAVIGATION
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setStyle(STYLE_NO_TITLE, 0)
    }

    override fun onCreateView(
        inflater: LayoutInflater,
        container: ViewGroup?,
        savedInstanceState: Bundle?
    ): View {
        _binding = SettingBinding.inflate(inflater, container, false)

        binding.channelsUrl.setText(SP.channelsUrl)
        binding.playerEngine.adapter = ArrayAdapter(
            requireContext(), R.layout.player_engine_item, listOf("MediaPlayer", "ExoPlayer")
        ).apply { setDropDownViewResource(R.layout.player_engine_dropdown_item) }
        binding.playerEngine.setSelection(if (SP.useExoPlayer) 1 else 0)
        binding.playerEngine.onItemSelectedListener = object : AdapterView.OnItemSelectedListener {
            override fun onItemSelected(parent: AdapterView<*>?, view: View?, position: Int, id: Long) {
                val exo = position == 1
                if (exo != SP.useExoPlayer) {
                    SP.useExoPlayer = exo
                    (activity as? MainActivity)?.restartPlaybackEngine()
                }
            }
            override fun onNothingSelected(parent: AdapterView<*>?) {}
        }
        binding.closeMobileSettings.apply {
            setOnClickListener { dismiss() }
        }
        binding.saveChannelsUrl.setOnClickListener {
            val url = binding.channelsUrl.text.toString().trim()
            if (url.isBlank()) {
                Toast.makeText(context, "请输入频道列表地址", Toast.LENGTH_SHORT).show()
            } else {
                (activity as? MainActivity)?.apply {
                    reloadChannels(url) { success ->
                        this@SettingFragment.context?.let {
                            Toast.makeText(
                                it,
                                if (success) "频道已更新" else "加载失败，原地址未变",
                                Toast.LENGTH_SHORT
                            ).show()
                        }
                    }
                }
            }
        }

        binding.switchChannelReversal.run {
            isChecked = SP.channelReversal
            setOnCheckedChangeListener { _, isChecked ->
                SP.channelReversal = isChecked
            }
        }

        binding.switchChannelNumberInput.run {
            isChecked = SP.channelNumberInput
            setOnCheckedChangeListener { _, isChecked ->
                SP.channelNumberInput = isChecked
            }
        }

        binding.switchBootStartup.run {
            isChecked = SP.bootStartup
            setOnCheckedChangeListener { _, isChecked ->
                SP.bootStartup = isChecked
            }
        }

        return binding.root
    }

    override fun onDestroyView() {
        super.onDestroyView()
        _binding = null
    }

    companion object {
        const val TAG = "SettingFragment"
    }
}
