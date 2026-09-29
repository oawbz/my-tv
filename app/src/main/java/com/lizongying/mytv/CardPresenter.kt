package com.lizongying.mytv

import android.view.ContextThemeWrapper
import android.view.ViewGroup
import android.widget.ImageView
import androidx.leanback.widget.ImageCardView
import androidx.leanback.widget.Presenter
import androidx.core.content.ContextCompat
import com.bumptech.glide.Glide
import com.bumptech.glide.load.engine.DiskCacheStrategy
import com.lizongying.mytv.models.TVViewModel

class CardPresenter : Presenter() {
    override fun onCreateViewHolder(parent: ViewGroup): ViewHolder {
        val card = ImageCardView(ContextThemeWrapper(parent.context, R.style.CustomImageCardTheme))
        card.isFocusable = true
        card.isFocusableInTouchMode = true
        return ViewHolder(card)
    }

    override fun onBindViewHolder(viewHolder: ViewHolder, item: Any?) {
        val tv = (item as TVViewModel).getTV()
        val card = viewHolder.view as ImageCardView
        card.titleText = tv.title
        card.contentText = tv.channel
        card.setMainImageDimensions(300, 101)
        card.mainImageView?.let { image ->
            image.setBackgroundColor(
                ContextCompat.getColor(image.context, R.color.channel_logo_background)
            )
            if (tv.logo.isNotEmpty()) {
                Glide.with(image).load(tv.logo)
                    .diskCacheStrategy(DiskCacheStrategy.NONE)
                    .centerInside().into(image)
            } else {
                Glide.with(image).clear(image)
            }
        }
        card.setBackgroundColor(
            ContextCompat.getColor(card.context, R.color.channel_text_background)
        )
        card.setMainImageScaleType(ImageView.ScaleType.CENTER_INSIDE)
    }

    override fun onUnbindViewHolder(viewHolder: ViewHolder) {
        (viewHolder.view as ImageCardView).mainImageView?.let { image ->
            Glide.with(image).clear(image)
        }
    }
}
