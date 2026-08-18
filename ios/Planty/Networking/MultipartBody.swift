import Foundation

/// Minimal multipart/form-data writer for the photo upload. Foundation has no
/// builder, and a photo is the one thing this app must never lose.
struct MultipartBody {
    let boundary: String
    private var data = Data()

    init(boundary: String = "planty.\(UUID().uuidString)") {
        self.boundary = boundary
    }

    var contentType: String { "multipart/form-data; boundary=\(boundary)" }

    mutating func appendField(name: String, value: String) {
        append("--\(boundary)\r\n")
        append("Content-Disposition: form-data; name=\"\(name)\"\r\n\r\n")
        append("\(value)\r\n")
    }

    mutating func appendFile(name: String, filename: String, contentType: String, data payload: Data) {
        append("--\(boundary)\r\n")
        append("Content-Disposition: form-data; name=\"\(name)\"; filename=\"\(filename)\"\r\n")
        append("Content-Type: \(contentType)\r\n\r\n")
        data.append(payload)
        append("\r\n")
    }

    func finished() -> Data {
        var closed = data
        closed.append(Data("--\(boundary)--\r\n".utf8))
        return closed
    }

    private mutating func append(_ string: String) {
        data.append(Data(string.utf8))
    }
}
