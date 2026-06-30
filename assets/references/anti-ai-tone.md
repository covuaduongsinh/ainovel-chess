# Tiêu chí khử giọng AI

Đây là kho tiêu chí "giọng AI" dùng chung cho writer và editor: writer khi viết né tránh tất cả các mẫu dưới đây, editor khi thẩm định chiều aesthetic kiểm tra từng mục và **trích nguyên văn** làm chứng.

> Phần có thể liệt kê máy móc (dấu gạch ngang, câu sáo cố định, từ lặp mòn tần suất cao) đã được `working_memory.user_rules.structured` kiểm tra bắt buộc khi commit; bài này chuyên về **phán đoán ngữ nghĩa không cơ học hóa được**. Hai bên bổ trợ: tầng cơ học bắt bề mặt, bài này bắt chất.

> Lưu ý: đây là BẢN NHÁP tiếng Việt (chuyển từ bản gốc viết cho văn tiếng Trung). Khái niệm chuyển được; ví dụ đã thay bằng mẫu sáo tiếng Việt, cần tinh chỉnh thêm bằng ngữ liệu thật.

## Một, giọng AI ở cấu trúc

- **Cấu trúc ba vế / điệp tam liên**: dùng liền ba câu ngắn hoặc vế đối xứng để "tạo thế" ("anh không còn do dự, không còn lùi bước, không còn ngoảnh lại"). Cách sửa: giữ một vế mạnh nhất, các vế còn lại tách thành hành động hoặc chi tiết.
- **Chất đống câu đối xứng đều tăm tắp**: mỗi đoạn dài bằng nhau, kiểu câu giống hệt, đọc như danh sách. Cách sửa: câu dài câu ngắn xen kẽ, cho nhịp có hơi thở.
- **Tiểu tiêu đề đánh số trong chương / cắt bằng `##`**: phần chính xuất hiện `Một` `Hai` `Ba` hoặc phân đoạn `##`/`###`. Cách sửa: chỉ giữ tiêu đề chương, chuyển cảnh dùng dòng trống để chuyển tiếp tự nhiên.

## Hai, giọng AI ở dùng từ

- **Chất đống thành ngữ Hán-Việt**: nhồi nhiều thành ngữ vào một đoạn để thay miêu tả ("kinh tâm động phách, hiểm tượng hoàn sinh, thiên quân nhất phát"). Cách sửa: dùng một hành động hoặc hình ảnh cụ thể thay cho một chuỗi thành ngữ.
- **Câu so sánh sáo**: các khuôn cố định "tựa như… ", "như thể… ", "phảng phất như… " lặp đi lặp lại. Cách sửa: đổi sang động từ chính xác hoặc bản thể tươi mới, hoặc tả mộc trực tiếp.
- **Tật lượng từ / tật hư từ**: "một tia", "một thoáng", "một làn" kèm cảm xúc; "không khỏi", "bỗng nhiên", "bất giác", "phảng phất" làm câu cửa miệng. Cách sửa: bỏ từ đệm, để hành động xảy ra trực tiếp ("anh cười", không phải "anh không khỏi khẽ nhếch lên một nụ cười").
- **Đại từ trừu tượng**: "ở một mức độ nào đó", "đáng chú ý là", "không hiểu vì sao", "khó nói thành lời" — người kể đang tóm tắt thay độc giả. Cách sửa: bỏ đi, nhường phán đoán cho sự thật cụ thể.
- **Câu định nghĩa đối lập**: "cái anh muốn không phải X, mà là Y", "đây không phải kết thúc, mà là khởi đầu" — kiểu dùng phủ định + chuyển ý để "điểm đề" lặp lại nhiều lần. Cách sửa: dùng một hành động hoặc lựa chọn cụ thể để trình bày trực tiếp, không dựa vào khuôn câu để tạo cảm giác câu vàng.

## Ba, giọng AI ở miêu tả

- **Khái quát trừu tượng thay cho ngũ giác cụ thể**: các khái quát dán nhãn kiểu "không khí ngột ngạt", "bầu không khí căng thẳng". Cách sửa: cho một chi tiết cụ thể cảm nhận được bằng xúc giác/khứu giác/thính giác (hơn là thuần thị giác).
- **Dán nhãn cảm xúc**: viết thẳng "anh rất căng thẳng/giận dữ/đau buồn". Cách sửa: dùng phản ứng cơ thể và lựa chọn để thể hiện ("đốt ngón tay trắng bệch", "cổ họng nghẹn lại"), không gọi tên cảm xúc.

## Bốn, giọng AI ở hội thoại

- **Nhân vật đồng nhất hóa**: bỏ dấu chỉ người nói thì không phân biệt được ai đang nói — ai cũng cùng kiểu câu, dùng từ, tầng văn hóa như nhau. Cách sửa: cho mỗi nhân vật độ dài câu, câu cửa miệng, tỉ lệ hàm ý ngầm ổn định riêng.
- **Giải thích động cơ quá mức**: nhân vật phơi bày hết tâm lý của mình, hoặc người kể bám theo giải thích "anh nói vậy là vì…". Cách sửa: để động cơ giấu trong lựa chọn và hàm ý, tin tưởng độc giả.
- **Giọng văn viết**: ai cũng nói câu hoàn chỉnh, chỉnh tề, kèm từ nối logic. Cách sửa: khẩu ngữ có ngắt quãng, lược bỏ, trả lời lệch câu hỏi.

## Năm, giọng AI ở nhịp và cảm xúc

- **Giao đãi mọi thứ đầy đủ**: mỗi hành động, nhân quả đều viết kín, không chừa khoảng tưởng tượng. Cách sửa: cái cần giấu thì giấu, dùng khoảng trống tạo lực theo đọc.
- **Cố thăng hoa / điểm đề ở kết**: cuối chương nâng lên thành chiêm nghiệm nhân sinh hoặc câu vàng chủ đề. Cách sửa: dừng ở một hình ảnh, lựa chọn hoặc dư âm cảm xúc cụ thể, đừng tóm tắt ý nghĩa thay độc giả.
