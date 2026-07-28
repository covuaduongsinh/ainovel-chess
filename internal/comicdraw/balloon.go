package comicdraw

import (
	"math"
	"math/rand"
)

// Các hàm dưới đây sinh hình bong bóng theo THAM SỐ, luôn có đối số inflate.
//
// Nhờ tham số hoá mà không cần stroker: muốn nét viền dày w thì tô path với inflate=+w
// bằng màu nét, rồi tô lại với inflate=0 bằng màu ruột. Hai lần tô đều qua rasterizer nên
// cả mép ngoài lẫn mép trong đều khử răng cưa đúng.
//
// Đuôi bong bóng được phát vào CÙNG một Path với thân: non-zero winding tự hợp nhất hai
// contour, nên chỗ nối liền mạch mà không phải tính giao điểm.

// ellipsePath dựng thân bong bóng thoại.
func ellipsePath(cx, cy, rx, ry, inflate float64) *Path {
	p := &Path{}
	return p.AddEllipse(cx, cy, rx, ry, inflate)
}

// spikyPath dựng bong bóng hét: đa giác răng cưa xen kẽ bán kính ngoài/trong.
// Biên độ răng dao động nhẹ theo seed để không nhìn như bánh răng cơ khí, nhưng vẫn
// XÁC ĐỊNH theo seed — dựng lại trang cho ra đúng hình cũ, nhờ đó golden test mới ổn định.
func spikyPath(cx, cy, rx, ry, inflate float64, seed int64) *Path {
	rx, ry = rx+inflate, ry+inflate
	perim := math.Pi * (3*(rx+ry) - math.Sqrt((3*rx+ry)*(rx+3*ry)))
	n := int(math.Round(perim / (math.Max(rx, ry) * 0.42)))
	if n < 12 {
		n = 12
	}
	if n > 40 {
		n = 40
	}
	rng := rand.New(rand.NewSource(seed))
	p := &Path{}
	for i := 0; i < 2*n; i++ {
		ang := 2 * math.Pi * float64(i) / float64(2*n)
		f := 1.0
		if i%2 == 0 {
			f = 1.0 + (rng.Float64()-0.5)*0.12 // răng ngoài, dao động ±6%
		} else {
			f = 0.78
		}
		x, y := cx+math.Cos(ang)*rx*f, cy+math.Sin(ang)*ry*f
		if i == 0 {
			p.MoveTo(x, y)
		} else {
			p.LineTo(x, y)
		}
	}
	return p.Close()
}

// cloudPath dựng bong bóng độc thoại: một ellipse trung tâm + N thuỳ tròn quanh chu vi,
// tất cả là contour riêng trong cùng một Path để winding hợp nhất thành hình mây.
func cloudPath(cx, cy, rx, ry, inflate float64) *Path {
	rx, ry = rx+inflate, ry+inflate
	p := &Path{}
	p.AddEllipse(cx, cy, rx*0.82, ry*0.78, 0)
	n := int(math.Round(math.Max(8, math.Min(16, (rx+ry)/26))))
	for i := 0; i < n; i++ {
		ang := 2 * math.Pi * float64(i) / float64(n)
		lx, ly := cx+math.Cos(ang)*rx*0.80, cy+math.Sin(ang)*ry*0.76
		lobe := math.Min(rx, ry) * 0.36
		p.AddEllipse(lx, ly, lobe, lobe, 0)
	}
	return p
}

// dashedEllipsePath dựng viền đứt cho bong bóng thì thầm: phát các cung rời rạc dưới dạng
// những contour hình nêm mỏng. Không cần engine nét đứt.
func dashedEllipsePath(cx, cy, rx, ry, thickness float64) *Path {
	p := &Path{}
	const dashes = 22
	for i := 0; i < dashes; i++ {
		if i%2 == 1 {
			continue
		}
		a0 := 2 * math.Pi * float64(i) / float64(dashes)
		a1 := 2 * math.Pi * float64(i+1) / float64(dashes)
		steps := 4
		// Mép ngoài
		for s := 0; s <= steps; s++ {
			a := a0 + (a1-a0)*float64(s)/float64(steps)
			x, y := cx+math.Cos(a)*(rx+thickness/2), cy+math.Sin(a)*(ry+thickness/2)
			if s == 0 {
				p.MoveTo(x, y)
			} else {
				p.LineTo(x, y)
			}
		}
		// Mép trong, đi ngược lại
		for s := steps; s >= 0; s-- {
			a := a0 + (a1-a0)*float64(s)/float64(steps)
			p.LineTo(cx+math.Cos(a)*(rx-thickness/2), cy+math.Sin(a)*(ry-thickness/2))
		}
		p.Close()
	}
	return p
}

// tailPath dựng đuôi bong bóng trỏ từ thân về phía điểm tx,ty.
//
// Đỉnh đuôi được kéo lùi khỏi đích một chút để không chạm vào miệng nhân vật; hai cạnh
// uốn nhẹ bằng QuadTo với điểm điều khiển lệch vuông góc — chính cái vẩy cong đó tạo ra
// dáng đuôi truyện tranh quen thuộc thay vì một cái nêm cứng đờ.
//
// shorten rút ngắn đỉnh đuôi dọc theo hướng đi. Đây là tham số BẮT BUỘC để nét viền hiện
// ra ở đầu đuôi: nếu contour ruột dài bằng contour nét thì ruột sẽ phủ kín nét, đuôi in ra
// bị trắng nhợt ở mũi thay vì có viền bao quanh.
func tailPath(cx, cy, rx, ry, tx, ty, width, shorten float64) *Path {
	dx, dy := tx-cx, ty-cy
	dist := math.Hypot(dx, dy)
	if dist < 1 {
		return &Path{}
	}
	ux, uy := dx/dist, dy/dist
	px, py := -uy, ux // pháp tuyến

	// Hai chân đuôi nằm trên biên ellipse, lệch ±width/2 quanh hướng đi.
	baseAng := math.Atan2(uy*rx, ux*ry)
	spread := width / 2 / math.Max(rx, ry)
	a1, a2 := baseAng-spread, baseAng+spread
	b1x, b1y := cx+math.Cos(a1)*rx, cy+math.Sin(a1)*ry
	b2x, b2y := cx+math.Cos(a2)*rx, cy+math.Sin(a2)*ry

	// Đỉnh lùi khỏi đích 6% quãng đường, rồi lùi thêm shorten.
	apexLen := math.Max(dist*0.35, dist*0.94-shorten)
	apexX, apexY := cx+ux*apexLen, cy+uy*apexLen

	bend := dist * 0.15
	p := &Path{}
	p.MoveTo(b1x, b1y)
	p.QuadTo(b1x+ux*apexLen*0.5+px*bend, b1y+uy*apexLen*0.5+py*bend, apexX, apexY)
	p.QuadTo(b2x+ux*apexLen*0.5-px*bend, b2y+uy*apexLen*0.5-py*bend, b2x, b2y)
	return p.Close()
}

// thoughtTailPath dựng đuôi bong bóng độc thoại: ba bong bóng tròn nhỏ dần.
func thoughtTailPath(cx, cy, rx, ry, tx, ty float64) *Path {
	dx, dy := tx-cx, ty-cy
	dist := math.Hypot(dx, dy)
	if dist < 1 {
		return &Path{}
	}
	ux, uy := dx/dist, dy/dist
	p := &Path{}
	for i, f := range []float64{0.45, 0.68, 0.88} {
		r := math.Min(rx, ry) * []float64{0.30, 0.20, 0.12}[i]
		p.AddEllipse(cx+ux*dist*f, cy+uy*dist*f, r, r, 0)
	}
	return p
}

// balloonBody dựng thân bong bóng (chưa gồm đuôi) theo loại.
func balloonBody(kind BalloonKind, cx, cy, rx, ry, inflate float64, corner float64, seed int64) *Path {
	switch kind {
	case BalloonShout:
		return spikyPath(cx, cy, rx, ry, inflate, seed)
	case BalloonThought:
		return cloudPath(cx, cy, rx, ry, inflate)
	case BalloonBox:
		p := &Path{}
		return p.AddRoundRect(cx-rx, cy-ry, 2*rx, 2*ry, corner, inflate)
	default: // BalloonSpeech, BalloonWhisper
		return ellipsePath(cx, cy, rx, ry, inflate)
	}
}
