import Foundation

public enum NativeTraceContext {
    public static func attach(to request: inout URLRequest) -> UUID? {
        guard request.value(forHTTPHeaderField: "traceparent") == nil else { return nil }
        let id = UUID()
        let traceID = id.uuidString.replacingOccurrences(of: "-", with: "").lowercased()
        request.setValue("00-\(traceID)-\(traceID.suffix(16))-01", forHTTPHeaderField: "traceparent")
        return id
    }
}
